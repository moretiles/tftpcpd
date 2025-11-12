package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

func serverInit() error {
	var err error

	// Increment the number of consumers attached to the row with a matching filename and the greatest uploadCompleted value returning that row. The only parameter is filename.
	reserveStatementRead, err = db.Prepare(`SELECT * FROM files WHERE
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename )
        LIMIT 1;`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Increment the number of consumers attached to the row with a matching filename and the greatest uploadCompleted value. The only parameter is filename.
	reserveStatementWrite, err = db.Prepare(`UPDATE files SET consumers = consumers + 1 WHERE
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename );`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Decrement if greater than 0 the number of consumers attached to the row with a matching filename and the greatest uploadCompleted value returning that row. The only parameter is filename.
	releaseStatementRead, err = db.Prepare(`SELECT * FROM files WHERE
        consumers != 0 AND
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename )
        LIMIT 1;`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Decrement if greater than 0 the number of consumers attached to the row with a matching filename and the greatest uploadCompleted value. The only parameter is filename.
	releaseStatementWrite, err = db.Prepare(`UPDATE files SET consumers = consumers - 1 WHERE
        consumers != 0 AND
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename );`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Create entry for filename at uploadedStarted where those are the parameters. uploadCompleted and consumers are 0 by default.
	prepareStatement, err = db.Prepare(`INSERT INTO files(filename, uploadStarted, uploadCompleted, consumers) VALUES (?, ?, 0, 0);`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Delete row created at the beginning of the upload because it failed. Parameters are filename and uploadStarted.
	overwriteFailureStatement, err = db.Prepare(`DELETE FROM files WHERE filename = ? AND uploadStarted = ?;`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	// Update row created at the beginning of the upload to reflect its success. Parameters are uploadCompleted, filename, and uploadStarted.
	overwriteSuccessStatement, err = db.Prepare(`UPDATE files SET uploadCompleted = ? WHERE filename = ? AND uploadStarted = ?;`)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", cfg.address))
		return err
	}

	return nil
}

func serverRoutine(childToParent chan<- Signal, parentToChild <-chan Signal) {
	var (
		// 0xffff is maximum possible in-transit packet size with TFTP
		incoming []byte = make([]byte, 0xffff)
		sessions sync.WaitGroup
	)

	serverAddr, err := net.ResolveUDPAddr("udp", cfg.address)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Unable to resolve address: %v", serverAddr.String()))
		childToParent <- NewSignal(SignalTerminate, SignalRequest)
		<-parentToChild
		return
	}

	conn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		log <- newErrorEvent("SERVER", fmt.Sprintf("Unable to bind to address: %v", cfg.address))
		childToParent <- NewSignal(SignalTerminate, SignalRequest)
		<-parentToChild
		return
	}
	defer conn.Close()

	log <- newNormalEvent("SERVER", fmt.Sprintf("Server successfully bound to: %v", serverAddr.String()))

	// Prepare context and set prepared statements sessions goroutines need
	ctx, cancel := context.WithCancel(context.Background())

	for true {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := conn.ReadFromUDP(incoming)

		select {
		case sig := <-parentToChild:
			childToParent <- NewSignal(sig.Kind, SignalAccept)
			cancel()
			sessions.Wait()
			return
		default:
			// no need to stop
		}

		if os.IsTimeout(err) {
			continue
		} else if err != nil {
			childToParent <- NewSignal(SignalTerminate, SignalRequest)
			<-parentToChild
			cancel()
			sessions.Wait()
			return
		}

		incomingCopy := make([]byte, n)
		if copy(incomingCopy, incoming[:n]) != n {
			log <- newErrorEvent("SERVER", "Truncation error when reading message")
			continue
		}

		sessions.Go(func() { sessionRoutine(ctx, clientAddr, incomingCopy) })
	}
}

func sessionRoutine(ctx context.Context, destinationAddr *net.UDPAddr, bytes []byte) {
	session, err := newTftpSession(ctx, destinationAddr)
	if err != nil {
		log <- newErrorEvent(destinationAddr.String(), fmt.Sprintf("Failed to create tftpSession: %v", err))
	}
	defer session.Close()

	opcode, err := session.establish(bytes)
	if err != nil {
		session.errorMessage(errorCodeUndefined, fmt.Sprintf("%v", err))
		log <- newErrorEvent(destinationAddr.String(), fmt.Sprintf("Session routine failed to establish: %v", err))
		return
	}

	switch opcode {
	case opcodeReadByte:
		log <- newNormalEvent(session.destinationAddr.String(), fmt.Sprintf("Client began download: %v", session.filename))
		err = session.read()
	case opcodeWriteByte:
		log <- newNormalEvent(session.destinationAddr.String(), fmt.Sprintf("Client began upload: %v", session.filename))
		err = session.write()
	default:
		session.errorMessage(errorCodeUndefined, "Client requested invalid operation")
		log <- newErrorEvent(session.destinationAddr.String(), "Client requested invalid operation")
		return
	}

	// log error
	if err != nil {
		switch opcode {
		case opcodeReadByte:
			log <- newErrorEvent(destinationAddr.String(), fmt.Sprintf("Client failed download: %v", err))
		case opcodeWriteByte:
			log <- newErrorEvent(destinationAddr.String(), fmt.Sprintf("Client failed upload: %v", err))
		}
		session.errorMessage(errorCodeUndefined, fmt.Sprintf("%v", err))
		return
	}

	switch opcode {
	case opcodeReadByte:
		log <- newNormalEvent(destinationAddr.String(), fmt.Sprintf("Client completed download: %v", session.filename))
	case opcodeWriteByte:
		log <- newNormalEvent(destinationAddr.String(), fmt.Sprintf("Client completed upload: %v", session.filename))
	}
	return
}
