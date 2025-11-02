package main

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
)

func echoRoutine(demandTermination chan bool, confirmTermination chan bool) {
	// 0xffff is maximum possible in-transit packet size with TFTP
	var incoming []byte = make([]byte, 0xffff)

	// bind to cfg.address
	addr, err := net.ResolveUDPAddr("udp", cfg.address)
	if err != nil {
		log <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to resolve address: %v", cfg.address))

		demandTermination <- true
		<-confirmTermination

		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to bind to address: %v", cfg.address))

		demandTermination <- true
		<-confirmTermination

		return
	}
	defer conn.Close()

	for true {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := conn.ReadFromUDP(incoming)

		select {
		case <-demandTermination:
			confirmTermination <- true

			return
		default:
			// no need to stop
		}

		if os.IsTimeout(err) {
			continue
		}

		log <- newDebugEvent(addr.String(), fmt.Sprintf("Got bytes %v", incoming[:n]))
		message, err := BytesAsMessage(incoming[:n])
		if err != nil {
			fmt.Println("Error turning bytes into message")
			log <- newErrorEvent(addr.String(), "Error turning bytes into message")
			continue
		}

		log <- newDebugEvent(addr.String(), fmt.Sprintf("Got message of type %v", reflect.TypeOf(message)))
	}
}

func serverRoutine(demandTermination chan bool, confirmTermination chan bool) {
	var (
		// 0xffff is maximum possible in-transit packet size with TFTP
		incoming []byte = make([]byte, 0xffff)
		sessions sync.WaitGroup
	)

	serverAddr, err := net.ResolveUDPAddr("udp", cfg.address)
	if err != nil {
		log <- newErrorEvent(serverAddr.String(), fmt.Sprintf("Unable to resolve address: %v", serverAddr.String()))
		demandTermination <- true
		<-confirmTermination
		return
	}

	conn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		log <- newErrorEvent(serverAddr.String(), fmt.Sprintf("Unable to bind to address: %v", cfg.address))
		demandTermination <- true
		<-confirmTermination
		return
	}
	defer conn.Close()

	for true {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := conn.ReadFromUDP(incoming)

		select {
		case <-demandTermination:
			confirmTermination <- true
			sessions.Wait()
			return
		default:
			// no need to stop
		}

		if os.IsTimeout(err) {
			continue
		} else if err != nil {
			demandTermination <- true
			<-confirmTermination
			sessions.Wait()
			return
		}

		incomingCopy := make([]byte, n)
		if copy(incomingCopy, incoming[:n]) != n {
			log <- newErrorEvent(clientAddr.String(), "Truncation error when reading message")
			continue
		}
		//fmt.Println(incomingCopy)

		//select {
		//case terminatedSession, isOpen := <-terminatedSessions:
		//if !isOpen {
		//log <- newErrorEvent(clientAddr.String(), "Listener reached unrecoverable error when managing index of existing sessions, waiting for existing connections to die")
		//demandTermination <- true
		//<-confirmTermination
		//sessions.Wait()
		//return
		//}

		//fmt.Println("Yes terminated sessions")
		//delete(activeSessions, terminatedSession)
		//default:
		//fmt.Println("No terminated sessions")
		//}

		//fmt.Println("client =", clientAddr.String())
		//fmt.Println("There are", len(activeSessions), "active sessions")
		//session, exists := activeSessions[clientAddr.String()]
		//if !exists {
		//session = make(chan []byte)
		//activeSessions[clientAddr.String()] = session
		// not implemented yet
		//sessions.Go(func() { sessionRoutine(clientAddr, session, terminatedSessions) })
		//}
		//session <- incomingCopy

		sessions.Go(func() { sessionRoutine(clientAddr, incomingCopy) })
	}
}

func sessionRoutine(destinationAddr *net.UDPAddr, bytes []byte) {
	session, err := newTftpSession(destinationAddr)
	if err != nil {
		log <- newErrorEvent(destinationAddr.String(), fmt.Sprintf("Failed to create tftpSession for %v", err))
	}
	defer session.Close()

	opcode, err := session.establish(bytes)
	if err != nil {
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
