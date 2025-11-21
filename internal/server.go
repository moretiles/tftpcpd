package internal

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

func ServerInit() error {
	var err error

	// Find the row with a matching filename and the greatest uploadCompleted value returning that row. The only parameter is filename.
	ReserveStatementSelect, err = DB.Prepare(`SELECT * FROM files WHERE
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename )
        LIMIT 1;`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Increment the number of consumers attached to the row with a matching filename and the greatest uploadCompleted value. The only parameter is filename.
	ReserveStatementUpdate, err = DB.Prepare(`UPDATE files SET consumers = consumers + 1 WHERE
        filename = ? AND
        (filename, uploadCompleted, consumers) IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename );`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Find all rows with this filename that are out of date and not being consumed.
	ReleaseStatementSelect, err = DB.Prepare(`SELECT * FROM files WHERE
            filename = ? AND
            uploadCompleted != 0 AND
            consumers = 0 AND
            (rowid, filename, uploadCompleted) NOT IN ( SELECT rowid, filename, MAX(uploadCompleted) FROM files GROUP BY filename );`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Decrement if greater than 0 the number of consumers attached to the row with a matching filename and the associated uploadStarted value. The only parameters are filename and uploadStarted time.
	ReleaseStatementUpdate, err = DB.Prepare(`UPDATE files SET consumers = consumers - 1 WHERE
        consumers != 0 AND
        filename = ? AND
        uploadStarted = ?;`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Clear files with same filename if out of date and not being accessed. Deletes row if number of consumers is 0, file is not being uploaded, filename is the same, and this is not the newest version of this file. The only parameters are filename.
	ReleaseStatementDelete, err = DB.Prepare(`DELETE FROM files WHERE
        consumers == 0 AND
        uploadCompleted != 0 AND
        filename = ? AND
        (filename, uploadCompleted, consumers) NOT IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename );`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Create entry for filename at uploadedStarted where those are the parameters. uploadCompleted and consumers are 0 by default.
	PrepareStatement, err = DB.Prepare(`INSERT INTO files(filename, uploadStarted, uploadCompleted, consumers) VALUES (?, ?, 0, 0);`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Delete row created at the beginning of the upload because it failed. Parameters are filename and uploadStarted.
	OverwriteFailureStatement, err = DB.Prepare(`DELETE FROM files WHERE filename = ? AND uploadStarted = ?;`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Find all rows with this filename that are out of date and not being consumed.
	OverwriteSuccessSelect, err = DB.Prepare(`SELECT * FROM files WHERE
            filename = ? AND
            uploadCompleted != 0 AND
            consumers = 0 AND
            (rowid, filename, uploadCompleted) NOT IN ( SELECT rowid, filename, MAX(uploadCompleted) FROM files GROUP BY filename );`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Update row created at the beginning of the upload to reflect its success. Parameters are uploadCompleted, filename, and uploadStarted.
	OverwriteSuccessUpdate, err = DB.Prepare(`UPDATE files SET uploadCompleted = ? WHERE filename = ? AND uploadStarted = ?;`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	// Clear files with same filename if out of date and not being accessed. Deletes row if number of consumers is 0, file is not being uploaded, filename is the same, and this is not the newest version of this file. The only parameters are filename.
	OverwriteSuccessDelete, err = DB.Prepare(`DELETE FROM files WHERE
        consumers == 0 AND
        uploadCompleted != 0 AND
        filename = ? AND
        (filename, uploadCompleted, consumers) NOT IN ( SELECT filename, MAX(uploadCompleted), consumers FROM files GROUP BY filename );`)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Failed setup to talk to internal database: %v", Cfg.Address))
		return err
	}

	return nil
}

func ServerRoutine(childToParent chan<- Signal, parentToChild <-chan Signal) {
	var (
		// 0xffff is maximum possible in-transit packet size with TFTP
		incoming []byte = make([]byte, 0xffff)
		sessions sync.WaitGroup
	)

	serverAddr, err := net.ResolveUDPAddr("udp", Cfg.Address)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Unable to resolve address: %v", serverAddr.String()))
		childToParent <- NewSignal(SignalTerminate, SignalRequest)
		<-parentToChild
		return
	}

	conn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		Log <- NewErrorEvent("SERVER", fmt.Sprintf("Unable to bind to address: %v", Cfg.Address))
		childToParent <- NewSignal(SignalTerminate, SignalRequest)
		<-parentToChild
		return
	}
	defer conn.Close()

	Log <- NewNormalEvent("SERVER", fmt.Sprintf("Server successfully bound to: %v", serverAddr.String()))

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
			Log <- NewErrorEvent("SERVER", "Truncation error when reading message")
			continue
		}

		sessions.Go(func() { sessionRoutine(ctx, clientAddr, incomingCopy) })
	}
}

func sessionRoutine(ctx context.Context, destinationAddr *net.UDPAddr, bytes []byte) {
	var err error
	var destination *net.UDPConn

	destination, err = net.DialUDP("udp", nil, destinationAddr)
	if err != nil {
		Log <- NewErrorEvent(destinationAddr.String(), fmt.Sprintf("Failed to create tftpSession: %v", err))
		return
	}
	session, err := NewTftpSession(ctx, destination)
	if err != nil {
		Log <- NewErrorEvent(destinationAddr.String(), fmt.Sprintf("Failed to create tftpSession: %v", err))
		return
	}
	defer session.Close()

	operation, err := session.Accept(bytes)
	if err != nil {
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		Log <- NewErrorEvent(destinationAddr.String(), fmt.Sprintf("Session routine failed to accept: %v", err))
		return
	}

	switch operation {
	case ReadAsServer:
		Log <- NewNormalEvent(session.DestinationAddr.String(), fmt.Sprintf("Client began download: %v", session.Filename))
		err = session.ReadAsServer()
	case WriteAsServer:
		Log <- NewNormalEvent(session.DestinationAddr.String(), fmt.Sprintf("Client began upload: %v", session.Filename))
		err = session.WriteAsServer()
	default:
		session.ErrorMessage(ErrorCodeUndefined, "Client requested invalid operation")
		Log <- NewErrorEvent(session.DestinationAddr.String(), "Client requested invalid operation")
		return
	}

	// log error
	if err != nil {
		switch operation {
		case ReadAsServer:
			Log <- NewErrorEvent(destinationAddr.String(), fmt.Sprintf("Client failed download: %v", err))
		case WriteAsServer:
			Log <- NewErrorEvent(destinationAddr.String(), fmt.Sprintf("Client failed upload: %v", err))
		}
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		return
	}

	switch operation {
	case ReadAsServer:
		Log <- NewNormalEvent(destinationAddr.String(), fmt.Sprintf("Client completed download: %v", session.Filename))
	case WriteAsServer:
		Log <- NewNormalEvent(destinationAddr.String(), fmt.Sprintf("Client completed upload: %v", session.Filename))
	}
	return
}
