package main

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type tftpSession struct {
	// used for connection
	destinationAddr *net.UDPAddr
	destination     *net.UDPConn
	buf             []byte

	// set when opening file
	file     *os.File
	unixNano int64

	// set when connection established
	opcode   uint16
	filename string
	mode     string
	options  map[string]string

	// set when negotiating options
	blockSize    uint16
	timeout      time.Duration
	transferSize uint64
	windowSize   uint16

	// updated upon acknowledgements
	blockNumber           uint16
	totalBytesTransferred uint32
	lastValidMessage      any
	mostRecentMessage     any
}

func newTftpSession(destination *net.UDPAddr) (tftpSession, error) {
	var err error
	var session tftpSession

	session.destinationAddr = destination
	session.destination, err = net.DialUDP("udp", nil, session.destinationAddr)
	if err != nil {
		return tftpSession{}, err
	}
	session.buf = make([]byte, 512)

	session.options = make(map[string]string)

	session.blockSize = 512
	session.timeout = time.Second * 5
	session.transferSize = 0 // 0 if unknown
	session.windowSize = 512

	return session, nil
}

func (session *tftpSession) lastSentMessageType() uint16 {
	if session.buf == nil || len(session.buf) < 2 {
		return opcodeInvalid
	}

	return uint16(session.buf[1])
}

// add self to list of readers for the most recent version of filename
func (session *tftpSession) reserve() (int64, *os.File, error) {
	// TODO:
	// use database to manage multiple versions

	unixMicro := time.Now().UnixMicro()
	file, err := os.Open(session.filename)
	return unixMicro, file, err
}

// remove self from list of readers attached what was the most recent version of filename at session.unixMicro
func (session *tftpSession) release(unixMicro int64) error {
	session.file.Close()

	// TODO:
	// use database to manage multiple versions

	return nil
}

// inform databse we want to begin writing a version of filename and get a time attached to it
func (session *tftpSession) prepare() (int64, *os.File, error) {
	unixMicro := time.Now().UnixMicro()

	// TODO:
	// use database to manage multiple versions

	file, err := os.Create(session.filename)
	return unixMicro, file, err
}

// inform database client succesfully uploaded entire file, mark it as available
func (session *tftpSession) overwrite(unixMicro int64, failed *error) error {
	if *failed != nil {
		// tell database this file upload failed
	}

	session.file.Close()

	// TODO:
	// use database to manage multiple versions

	return nil
}

func (session *tftpSession) readFile() error {
	if len(session.buf) <= 4 {
		return errors.New("Buffer size way too small to read")
	}

	// somewhat hacky way to avoid double copying
	// we let MessageAsBytes handle the first 4 bytes and handle the rest by reading
	err := MessageAsBytes(newDataMessage(session.blockNumber, nil), &session.buf)
	if err != nil {
		return err
	}

	session.buf = session.buf[:session.blockSize]
	n, err := session.file.Read(session.buf[4:])
	if n != len(session.buf) {
		return errors.New("Truncated")
	}
	if err != nil {
		return err
	}
	return nil
}

// assumption made is that session.buf already contains a data message as bytes
func (session *tftpSession) writeFile() error {
	if len(session.buf) <= 4 {
		return errors.New("Buffer size is too small to write")
	}

	n, err := session.file.Write(session.buf[4:])
	if n != len(session.buf[4:]) {
		return errors.New("Truncated")
	}
	if err != nil {
		return err
	}
	return nil
}

// send length bytes of session.buf to client
func (session *tftpSession) send() error {
	if session == nil || session.buf != nil {
		return nil
	}

	n, err := session.destination.Write(session.buf)
	if err != nil {
		return err
	}
	if n != len(session.buf) {
		return errors.New("Truncated network send")
	}

	return nil
}

// create and send data message to client
func (session *tftpSession) dataMessage() error {
	// session.buf used to store loaded data so special care taken to avoid unneeded secondary write
	// see session.fileRead to learn how data is put into proper order

	return session.send()
}

// create and send acknowledge message to client
func (session *tftpSession) acknowledgeMessage() error {
	err := MessageAsBytes(newAcknowledgeMessage(session.blockNumber), &session.buf)
	if err != nil {
		return err
	}
	if len(session.buf) != 2+2 {
		return errors.New("Truncated acknowledge message")
	}

	return session.send()
}

// create and send error message to client
func (session *tftpSession) errorMessage(code uint8, message string) error {
	err := MessageAsBytes(newErrorMessage(uint16(code), message), &session.buf)
	if err != nil {
		return err
	}
	if len(session.buf) != 2+2+len(message)+1 {
		return errors.New("Truncated error message")
	}

	return session.send()
}

// create and send option acknowledge message to client
func (session *tftpSession) optionAcknowledgeMessage() error {
	err := MessageAsBytes(newOptionAcknowledgeMessage(session.options), &session.buf)
	if err != nil {
		return err
	}
	expectedTotal := 0
	for key, value := range session.options {
		expectedTotal += len(key) + 1 + len(value) + 1
	}
	if len(session.buf) != 2+2+expectedTotal {
		return errors.New("Truncated option acknowledge message")
	}

	return session.send()
}

func (session *tftpSession) receive(client <-chan []byte) error {
	var err error

	// if we timeout then resend
	for _ = range 10 {
		select {
		case messageBytes := <-client:
			session.mostRecentMessage, err = BytesAsMessage(messageBytes)
			if err != nil {
				return errors.New("Invalid")
			}
			return nil
		case <-time.After(session.timeout):
			session.send()
		}

		return errors.New("Client connection (likely) dead")
	}

	return nil
}

// establish connection with client
// only valid messages client can use to initiate connection are readMessage and writeMessage
// check client options to see if they are valid
func (session *tftpSession) establish(client <-chan []byte) (uint16, error) {
	var err error

	if err = session.receive(client); err != nil {
		return opcodeInvalid, err
	}

	switch session.mostRecentMessage.(type) {
	case readMessage:
		session.opcode = opcodeReadByte
		session.filename = session.mostRecentMessage.(readMessage).filename
		session.mode = session.mostRecentMessage.(readMessage).mode
		err = session.updateOptions(session.mostRecentMessage.(readMessage).options)
		if err != nil {
			session.errorMessage(errorCodeUndefined, "One or more options contain invalid values")
			return opcodeInvalid, errors.New("One or more options contain invalid values")
		}

		//events <- newNormalEvent(destination, fmt.Sprintf("Attempt to download: %v", state.filename))

		if len(session.options) > 0 {
			if err = session.optionAcknowledgeMessage(); err != nil {
				return opcodeInvalid, errors.New("Unable to send option acknowledgement to read request")
			}
		}
	case writeMessage:
		session.opcode = opcodeWriteByte
		session.filename = session.mostRecentMessage.(writeMessage).filename
		session.mode = session.mostRecentMessage.(writeMessage).mode
		err = session.updateOptions(session.mostRecentMessage.(writeMessage).options)
		if err != nil {
			session.errorMessage(errorCodeUndefined, "One or more options contain invalid values")
			return opcodeInvalid, errors.New("One or more options contain invalid values")
		}

		//events <- newNormalEvent(destination, fmt.Sprintf("Attempt to upload: %v", state.filename))

		if len(session.options) > 0 {
			if session.optionAcknowledgeMessage() != nil {
				return opcodeInvalid, errors.New("Unable to send option acknowledgement to write request")
			}
		}
	default:
		session.errorMessage(errorCodeIllegalOperation, "Client requested invalid operation when opening connection")
		return opcodeInvalid, errors.New("Client requested invalid operation when opening connection")
	}

	return session.opcode, nil
}

func (session *tftpSession) read(client <-chan []byte) error {
	var err error
	var readEverything bool = false
	var unixMicro int64

	// If the client asked for options we may have already sent an options acknowledge message
	// If we have just sent an options acknowledgement message we need to operate on the client's acknowledgement message
	switch session.lastSentMessageType() {
	case opcodeOptionAcknowledgeByte:
		session.lastValidMessage = session.mostRecentMessage
		if err = session.receive(client); err != nil {
			return err
		}

		switch session.mostRecentMessage.(type) {
		case acknowledgeMessage:
			return errors.New("Did not acknowledge the server's attempt to open a connection with supplied options")
		}

		if session.mostRecentMessage.(acknowledgeMessage).blockNumber != 0 {
			return errors.New("Did not acknowledge the server's attempt to open a connection with supplied options")
		}
	default:
		// pass
	}

	// we expect that client will acknowledge first data block with 1
	session.blockNumber = 1

	// file may be less than blocksize
	if int(session.blockSize) != len(session.buf) {
		session.buf = make([]byte, session.blockSize)
	}

	unixMicro, session.file, err = session.reserve()
	if err != nil {
		return err
	}
	defer session.release(unixMicro)

	// for loop designed to deal with multiple possible client messages
	// if the client sends an acknowledgeMessage then we should send the next data message
	// if the client sends an errorMessage then we log it and return error
	// if the client sends anything else return error
	for !readEverything {
		err = session.readFile()
		if err != nil {
			if errors.Is(err, io.EOF) {
				readEverything = true
			} else {
				return errors.New("File read error")
			}
		}

		if session.dataMessage() != nil {
			return errors.New("Unable to send data to client")
		}

		session.lastValidMessage = session.mostRecentMessage
		if err = session.receive(client); err != nil {
			return err
		}

		// read until acknowledgement with correct blockNumber, handling gracefully retransmissions
		var i int
		for i = range 5 {
			switch session.mostRecentMessage.(type) {
			case acknowledgeMessage:
				if session.mostRecentMessage.(acknowledgeMessage).blockNumber == session.blockNumber {
					break
				} else if session.mostRecentMessage.(acknowledgeMessage).blockNumber < session.blockNumber {
					if err = session.receive(client); err != nil {
						return err
					}
				} else {
					return errors.New("Out of sync blockNumber")
				}
			case errorMessage:
				return errors.New(session.mostRecentMessage.(errorMessage).explanation)
			default:
				session.errorMessage(errorCodeIllegalOperation, "Client requested invalid operation during established connection")
				return errors.New("Client requested invalid operation during established connection")
			}
		}

		// check if client is sending bad acknowledgements or possible network issue
		if i >= 4 {
			// don't tell client so as to avoid further network issues
			// this is not technically a standard reason to terminate by RFC, but is reasonable
			return errors.New("Underlying network may be bad, many retransmiteed messages")
		} else {
			session.blockNumber += 1
		}
	}

	return nil
}

func (session *tftpSession) write(client <-chan []byte) error {
	var unixMicro int64
	var err error

	// we expect that client will acknowledge first data block with 1
	session.blockNumber = 1

	// file may be less than blocksize
	if int(session.blockSize) != len(session.buf) {
		session.buf = make([]byte, session.blockSize)
	}

	// get access to a file and associated time
	unixMicro, session.file, err = session.prepare()
	if err != nil {
		return err
	}
	defer session.overwrite(unixMicro, &err)

	// if the client sends an dataMessage then we should acknowledge it
	// if the client sends an errorMessage then we log it and return error
	// if the client sends anything else return error
	for true {
		session.lastValidMessage = session.mostRecentMessage
		if err = session.receive(client); err != nil {
			return err
		}

		// read until acknowledgement with correct blockNumber, handling gracefully retransmissions
		var i int
		for i = range 5 {
			switch session.mostRecentMessage.(type) {
			case dataMessage:
				if session.mostRecentMessage.(dataMessage).blockNumber == session.blockNumber {
					break
				} else if session.mostRecentMessage.(dataMessage).blockNumber < session.blockNumber {
					if err = session.receive(client); err != nil {
						return err
					}
				} else {
					err = errors.New("Out of sync blockNumber")
					return err
				}
			case errorMessage:
				err = errors.New(session.mostRecentMessage.(errorMessage).explanation)
				return err
			default:
				session.errorMessage(errorCodeIllegalOperation, "Client requested invalid operation during established connection")
				err = errors.New("Client requested invalid operation during established connection")
				return err
			}
		}

		// check if client is sending bad acknowledgements or possible network issue
		if i >= 4 {
			// don't tell client so as to avoid further network issues
			// this is not technically a standard reason to terminate by RFC, but is reasonable
			err = errors.New("Underlying network may be bad, many retransmiteed messages")
			return err
		} else {
			session.blockNumber += 1
		}

		// write to file
		session.writeFile()

		// acknowledge
		if err = session.acknowledgeMessage(); err != nil {
			return err
		}

		// less data sent than session.blockSize means we have reached end
		if 2+2+len(session.mostRecentMessage.(dataMessage).body) < int(session.blockSize) {
			break
		}
	}

	return nil
}

func (session *tftpSession) updateOptions(options map[string]string) error {
	for keyCased, valueAscii := range options {
		key := strings.ToLower(keyCased)
		valueInt, err := strconv.Atoi(valueAscii)
		if err != nil {
			panic(err)
		}

		switch key {
		case "blksize":
			if valueInt < 8 || valueInt > 65464 {
				// refer to original specification to learn about what should be done in this case
			} else {
				session.blockSize = uint16(valueInt)
				options["blksize"] = valueAscii
			}
		case "timeout":
			if valueInt < 1 || valueInt > 255 {
				// refer to original specification to learn about what should be done in this case
			} else {
				session.timeout = time.Second * time.Duration(valueInt)
				options["timeout"] = valueAscii
			}
		case "tsize":
			if valueInt <= 0 {
				// refer to original specification to learn about what should be done in this case
			} else {
				session.transferSize = uint64(valueInt)
				options["tsize"] = valueAscii
			}
		case "multicast":
			// not implement
			continue
		case "windowsize":
			continue
			//if valueInt < 1 || valueInt > 65535 {
			// refer to original specification to learn about what should be done in this case
			//}
			//session.windowSize = uint16(valueInt)
			//options["windowsize"] = valueAscii
		default:
			continue
		}
	}

	return nil
}
