package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
)

func echoRoutine(demandTermination chan bool, confirmTermination chan bool, events chan<- logEvent) {
	// 0xffff is maximum possible in-transit packet size with TFTP
	var incoming []byte = make([]byte, 0xffff)

	// bind to cfg.address
	addr, err := net.ResolveUDPAddr("udp", cfg.address)
	if err != nil {
		events <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to resolve address: %v", cfg.address))

		demandTermination <- true
		<-confirmTermination

		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		events <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to bind to address: %v", cfg.address))

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

		events <- newDebugEvent(addr.String(), fmt.Sprintf("Got bytes %v", incoming[:n]))
		message, err := BytesAsMessage(incoming[:n])
		if err != nil {
			fmt.Println("Error turning bytes into message")
			events <- newErrorEvent(addr.String(), "Error turning bytes into message")
			continue
		}

		events <- newDebugEvent(addr.String(), fmt.Sprintf("Got message of type %v", reflect.TypeOf(message)))
	}
}

func serverRoutine(demandTermination chan bool, confirmTermination chan bool, events chan<- logEvent) {
	var (
		// 0xffff is maximum possible in-transit packet size with TFTP
		incoming []byte = make([]byte, 0xffff)

		sessions           sync.WaitGroup
		terminatedSessions chan string              = make(chan string, 150)
		activeSessions     map[string](chan []byte) = make(map[string](chan []byte))
	)

	addr, err := net.ResolveUDPAddr("udp", cfg.address)
	if err != nil {
		events <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to resolve address: %v", cfg.address))
		demandTermination <- true
		<-confirmTermination
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		events <- newErrorEvent(addr.String(), fmt.Sprintf("Unable to bind to address: %v", cfg.address))
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
			events <- newErrorEvent(addr.String(), "Truncation error when reading message")
			continue
		}

		select {
		case terminatedSession, isOpen := <-terminatedSessions:
			if !isOpen {
				events <- newErrorEvent(addr.String(), "Listener reached unrecoverable error when managing index of existing sessions, waiting for existing connections to die")
				demandTermination <- true
				<-confirmTermination
				sessions.Wait()
				return
			}

			delete(activeSessions, terminatedSession)
		default:
			// continue
		}

		session, exists := activeSessions[addr.String()]
		if !exists {
			session = make(chan []byte)
			activeSessions[addr.String()] = session
			// not implemented yet
			sessions.Go(func() { sessionRoutine(addr, session, demandTermination) })
		}
		session <- incomingCopy
	}
}

func sessionRoutine(destinationAddr *net.UDPAddr, client <-chan []byte, terminate chan<- bool) {
	session, err := newTftpSession(destinationAddr)

	opcode, err := session.establish(client)
	defer func() {
		terminate <- true
	}()
	if err != nil {
		log <- newErrorEvent(destinationAddr.String(), "Session routine unexpected exit")
		return
	}

	switch opcode {
	case opcodeReadByte:
		err = session.read(client)
	case opcodeWriteByte:
		err = session.write(client)
	default:
		err = errors.New("Don't know this opcode")
	}

	// log error
	if err != nil {
		log <- newErrorEvent(destinationAddr.String(), "Session routine unexpected exit")
		return
	}

	return
}
