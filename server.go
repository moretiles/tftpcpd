package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type tftpState struct {
	// set before connection established
	destination net.UDPAddr

	// set when connection established
	opcode             uint16
	filename           string
	mode               string
	options            map[string]string
	acceptedAllOptions bool

	// values determined by options
	blockSize    uint16
	timeout      time.Duration
	transferSize uint64
	windowSize   uint16

	// updated upon acknowledgements
	blockNumber           uint16
	totalBytesTransferred uint32
}

func newTftpState(destination net.UDPAddr) tftpState {
	var state tftpState

	state.destination = destination
	state.blockSize = 512
	state.timeout = time.Second * 5
	state.transferSize = 0 // 0 if unknown
	state.windowSize = 512
	state.options = make(map[string]string)
	state.acceptedAllOptions = true

	return state
}

func (state *tftpState) updateOptions(options map[string]string) error {
	for keyCased, valueAscii := range options {
		key := strings.ToLower(keyCased)
		valueInt, err := strconv.Atoi(valueAscii)
		if err != nil {
			panic(err)
		}

		switch key {
		case "blksize":
			if valueInt < 8 || valueInt > 65464 {
				state.acceptedAllOptions = false
			} else {
				state.blockSize = uint16(valueInt)
				options["blksize"] = valueAscii
			}
		case "timeout":
			if valueInt < 1 || valueInt > 255 {
				state.acceptedAllOptions = false
			} else {
				state.timeout = time.Second * time.Duration(valueInt)
				options["timeout"] = valueAscii
			}
		case "tsize":
			if valueInt <= 0 {
				state.acceptedAllOptions = false
			} else {
				state.transferSize = uint64(valueInt)
				options["tsize"] = valueAscii
			}
		case "multicast":
			state.acceptedAllOptions = false
		case "windowsize":
			state.acceptedAllOptions = false

			//if valueInt < 1 || valueInt > 65535 {
			//panic(errors.New("invalid windowsize size"))
			//}
			//state.windowSize = uint16(valueInt)
			options["windowsize"] = valueAscii
		default:
			state.acceptedAllOptions = false
			return errors.New("Unrecognized key in options")
		}
	}

	return nil
}

func echoRoutine(demandTermination chan bool, confirmTermination chan bool, events chan<- logEvent) {
	var (
		// 0xffff is maximum possible in-transit packet size with TFTP
		incoming []byte = make([]byte, 0xffff)
	)

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
		terminatedSessions chan string
		activeSessions     map[string](chan []byte)
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
			//sessions.Go(func () { sessionRoutine(addr, session) } )
		}
		session <- incomingCopy
	}
}

/*
func sessionRoutine(destination net.UDPAddr, source chan []byte) {
    var err error
    var lastValidMessage any
    var mostRecentMessage any
    var outgoingPacket []byte
	state := newTftpState(destination)

	received := <-source
    mostRecentMessage = BytesAsMessage(received)

    switch mostRecentMessage.(type) {
    case readMessage:
        state.opcode = opcodeReadByte
        state.filename = mostRecentMessage.(readMessage).filename
        state.mode = mostRecentMessage.(readMessage).mode
        err = state.updateOptions(mostRecentMessage.(readMessage).options)
        if err != nil {
            panic(errors.New("Non-negotiable options"))
        }

        events <- newNormalEvent(destination, fmt.Sprintf("Attempt to download: %v", state.filename))
    case writeMessage:
        state.opcode = opcodeWriteByte
        state.filename = mostRecentMessage.(writeMessage).filename
        state.mode = mostRecentMessage.(writeMessage).mode
        err = state.updateOptions(mostRecentMessage.(writeMessage).options)
        if err != nil {
            panic(errors.New("Non-negotiable options"))
        }

        events <- newNormalEvent(destination, fmt.Sprintf("Attempt to upload: %v", state.filename))
    default:
        //terminate session
    }

    lastValidMessage = mostRecentMessage

    if(!state.acceptedAllOptions){
        switch lastValidMessage.(type) {
        case readMessage:
        case writeMessage:
        default:
            //should be impossible to reach here but either way terminate session
        }
    } else {
        switch lastValidMessage.(type) {
        case readMessage:
        case writeMessage:
        default:
            //should be impossible to reach here but either way terminate session
    }
    }

    for mostRecentMessage := range <- source {
        switch mostRecentMessage.(type):
        case readMessage:
            // error then end goroutine, client will then begin talking to a new session goroutine
        case writeMessage:
            // error then end gorutine, client will then begin talking to a new session goroutine
        case dataMessage:
            // all good if state.opcode == opcodeWriteByte
        case acknowledgeMessage:
            // all good if state.opcode == opcodeReadByte
        case errorMessage:
            // if serious error end goroutine
        case optionAcknowledgeMessage:
            switch lastValidMessage.(type) {
            case readMessage:
            case writeMessage:
            default:
                // error then end goroutine, client is confused
            }
        default:
            // error then end goroutine, client is confused
    }

    terminatedSession <- string(destination)
    return
}
*/
