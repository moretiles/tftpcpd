package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	dataPreambleLength = 4
)

type tftpSession struct {
	// used for connection
	destinationAddr *net.UDPAddr
	destination     *net.UDPConn
	sendBuf         []byte
	receiveBuf      []byte

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

	// Default values
	session.blockSize = 512
	session.timeout = time.Second * 5
	session.transferSize = 0 // 0 if unknown
	session.windowSize = 512

	session.destinationAddr = destination
	session.destination, err = net.DialUDP("udp", nil, session.destinationAddr)
	if err != nil {
		return tftpSession{}, err
	}
	// Add 4 to support the data message preamble
	session.sendBuf = make([]byte, session.blockSize+dataPreambleLength)
	session.receiveBuf = make([]byte, session.blockSize+dataPreambleLength)

	session.options = make(map[string]string)

	return session, nil
}

func (session *tftpSession) Close() error {
	return session.destination.Close()
}

func (session *tftpSession) lastSentMessageType() uint16 {
	opcodeLen := 2
	if session.sendBuf == nil || len(session.sendBuf) < opcodeLen {
		return opcodeInvalid
	}

	return uint16(session.sendBuf[1])
}

// add self to list of readers for the most recent version of filename
func (session *tftpSession) reserve() (int64, error) {
	// TODO:
	// Acquire write lock on global map

	// Temporary code unixMicro should be the most recent timestamp read
	unixMicro := time.Now().UnixMicro()
	// Eventual
	//unixMicro := database.GetMostRecent(given: filename)

	// Use the global map to get the linked list for the filename requested and append to the end the timestamp being reserved
	// Release lock

	file, err := os.Open(session.filename)
	if err != nil {
		return 0, err
	}
	session.file = file

	return unixMicro, err
}

// remove self from list of readers attached what was the most recent version of filename at session.unixMicro
func (session *tftpSession) release(unixMicro int64) error {
	session.file.Close()

	// TODO:
	// Acquire write lock on global map

	//iterate over linked list and delete first value that is unixMicro
	//should be at or near the start so quick lookup

	// Release lock

	return nil
}

// inform databse we want to begin writing a version of filename and get a time attached to it
func (session *tftpSession) prepare() (int64, error) {
	unixMicro := time.Now().UnixMicro()

	// TODO:
	// Tell database to create entry for filename with the writeStarted time equal to unixMicro and writeEnded time equal to 0

	file, err := os.Create(session.filename)
	if err != nil {
		return 0, err
	}

	session.file = file
	return unixMicro, err
}

// inform database client succesfully uploaded entire file, mark it as available
func (session *tftpSession) overwriteSuccess(unixMicro int64) error {
	err := session.file.Close()
	if err != nil {
		return err
	}

	// TODO:
	// Tell database to fetch the record where name = filename AND writeStarted = unixMicro, then update timeEnded to time.Now()

	return nil
}

// inform database client succesfully uploaded entire file, mark it as available
func (session *tftpSession) overwriteFailure(unixMicro int64) error {
	/*
	 * When err is nil then some error has prevented the file from being written as it should have been
	 * When err is os.ErrClosed then overwriteSuccess has already been called and all is good
	 * When err is any other error then something weird has happened
	 */
	fileError := session.file.Close()
	if errors.Is(fileError, os.ErrClosed) {
		return nil
	}

	// TODO:
	// Tell database to remove the record where name = filename AND writeStarted = unixMicro
	// Then delete the partially written file

	return fileError
}

func (session *tftpSession) readFile() error {
	if len(session.sendBuf) <= dataPreambleLength {
		return errors.New("Buffer size way too small to read")
	}

	// somewhat hacky way to avoid double copying
	// we let MessageAsBytes handle the first 4 bytes and handle the rest by reading
	err := MessageAsBytes(newDataMessage(session.blockNumber, nil), &session.sendBuf)
	if err != nil {
		return err
	}

	session.sendBuf = session.sendBuf[:session.blockSize+dataPreambleLength]
	n, err := session.file.Read(session.sendBuf[dataPreambleLength:])
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	// Need to adjust session.sendBuf slice length
	// Writing bytes to session.sendBuf[dataPreambleLen:] does not update session.sendBuf itself
	newSliceEnd := dataPreambleLength + n
	session.sendBuf = session.sendBuf[0:newSliceEnd]
	return err
}

// assumption made is that session.sendBuf already contains a data message as bytes
func (session *tftpSession) writeFile() error {
	body := session.mostRecentMessage.(dataMessage).body

	// Zero length data section means previous message was the last message containing file data
	if len(body) == 0 {
		return io.EOF
	}

	// somewhat hacky way to avoid double copying
	// We know that bytes 5 and onward must be actual data being sent
	n, err := session.file.Write(body)
	if n != len(body) {
		return errors.New("Truncated")
	}
	if err != nil {
		return err
	}

	// Short message means end of file
	if n < int(session.blockSize) {
		err = io.EOF
	}
	return err
}

// send length bytes of session.sendBuf to client
func (session *tftpSession) send() error {
	if session == nil || session.sendBuf == nil {
		return nil
	}

	n, err := session.destination.Write(session.sendBuf)
	if err != nil {
		return err
	}
	if n != len(session.sendBuf) {
		return errors.New("Truncated network send")
	}

	return nil
}

func (session *tftpSession) receive() error {

	// if we timeout then resend
	for _ = range 10 {
		// Add timeout
		session.receiveBuf = session.receiveBuf[:session.blockSize+dataPreambleLength]
		messageLength, addr, err := session.destination.ReadFromUDP(session.receiveBuf)
		session.receiveBuf = session.receiveBuf[:messageLength]
		if err != nil {
			return err
		}
		ip1, ip2 := addr.IP, session.destinationAddr.IP
		port1, port2 := addr.Port, session.destinationAddr.Port
		zone1, zone2 := addr.Zone, session.destinationAddr.Zone
		if !ip1.Equal(ip2) || port1 != port2 || zone1 != zone2 {
			return errors.New("Client changed ip/port... possible man in the middle attack?")
		}
		session.mostRecentMessage, err = BytesAsMessage(session.receiveBuf[:messageLength])
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New("Client connection (likely) dead")
}

// create and send data message to client
func (session *tftpSession) dataMessage() error {
	// session.sendBuf used to store loaded data so special care taken to avoid unneeded secondary write
	// see session.fileRead to learn how data is put into proper order

	return session.send()
}

// create and send acknowledge message to client
func (session *tftpSession) acknowledgeMessage() error {
	acknowledgeOpcodeLen := 2
	acknowledgeBlockNumLen := 2

	err := MessageAsBytes(newAcknowledgeMessage(session.blockNumber), &session.sendBuf)
	if err != nil {
		return err
	}
	if len(session.sendBuf) != acknowledgeOpcodeLen+acknowledgeBlockNumLen {
		return errors.New("Truncated acknowledge message")
	}

	return session.send()
}

// create and send error message to client
func (session *tftpSession) errorMessage(code uint8, message string) error {
	errorOpcodeLen := 2
	errorCodeLen := 2
	nullTerminatorLen := 1

	err := MessageAsBytes(newErrorMessage(uint16(code), message), &session.sendBuf)
	if err != nil {
		return err
	}
	if len(session.sendBuf) != errorOpcodeLen+errorCodeLen+len(message)+nullTerminatorLen {
		return errors.New("Truncated error message")
	}

	return session.send()
}

// create and send option acknowledge message to client
func (session *tftpSession) optionAcknowledgeMessage() error {
	optionAcknowledgeOpcodeLen := 2

	err := MessageAsBytes(newOptionAcknowledgeMessage(session.options), &session.sendBuf)
	if err != nil {
		return err
	}
	expectedTotal := 0
	for key, value := range session.options {
		expectedTotal += len(key) + 1 + len(value) + 1
	}
	if len(session.sendBuf) != optionAcknowledgeOpcodeLen+expectedTotal {
		return errors.New("Truncated option acknowledge message")
	}

	return session.send()
}

// establish connection with client
// only valid messages client can use to initiate connection are readMessage and writeMessage
// check client options to see if they are valid
func (session *tftpSession) establish(bytes []byte) (uint16, error) {
	var err error

	session.mostRecentMessage, err = BytesAsMessage(bytes)

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
		session.options = session.mostRecentMessage.(readMessage).options

	case writeMessage:
		session.opcode = opcodeWriteByte
		session.filename = session.mostRecentMessage.(writeMessage).filename
		session.mode = session.mostRecentMessage.(writeMessage).mode
		err = session.updateOptions(session.mostRecentMessage.(writeMessage).options)
		if err != nil {
			session.errorMessage(errorCodeUndefined, "One or more options contain invalid values")
			return opcodeInvalid, errors.New("One or more options contain invalid values")
		}
		session.options = session.mostRecentMessage.(writeMessage).options

	default:
		session.errorMessage(errorCodeIllegalOperation, "Client requested invalid operation when opening connection")
		return opcodeInvalid, errors.New("Client requested invalid operation when opening connection")
	}

	if len(session.options) > 0 {
		if session.optionAcknowledgeMessage() != nil {
			return opcodeInvalid, errors.New("Unable to send option acknowledgement to write request")
		}
	}

	return session.opcode, nil
}

func (session *tftpSession) read() error {
	var readEverything bool = false
	var unixMicro int64
	var err error

	// If the client asked for options we may have already sent an options acknowledge message
	// If we have just sent an options acknowledgement message we need to operate on the client's acknowledgement message
	switch session.lastSentMessageType() {
	case opcodeOptionAcknowledgeByte:
		session.lastValidMessage = session.mostRecentMessage
		if err = session.receive(); err != nil {
			return err
		}

		switch session.mostRecentMessage.(type) {
		case acknowledgeMessage:
			// pass
		default:
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

	// adjust buffer size because of blockSize
	if len(session.sendBuf) != int(dataPreambleLength+session.blockSize) {
		session.sendBuf = make([]byte, session.blockSize+dataPreambleLength)
	}
	if len(session.receiveBuf) != int(dataPreambleLength+session.blockSize) {
		session.receiveBuf = make([]byte, dataPreambleLength+session.blockSize)
	}

	unixMicro, err = session.reserve()
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
		if errors.Is(err, io.EOF) || len(session.sendBuf) < int(dataPreambleLength+session.blockSize) {
			readEverything = true
		} else if err != nil {
			return errors.New("File read error")
		}

		if session.dataMessage() != nil {
			return errors.New("Unable to send data to client")
		}

		// setup for receive loop
		session.lastValidMessage = session.mostRecentMessage
		var i int = 1
		var awaitingRequest bool = true

		// read until acknowledgement with correct blockNumber, handling gracefully retransmissions
		for awaitingRequest {
			// timeout after five bad messages
			if i > 5 {
				// don't tell client so as to avoid further network issues
				return errors.New("Underlying network may be bad, many retransmiteed messages")
			}

			// Get next potentially valid message
			if err = session.receive(); err != nil {
				return err
			}

			// Read messages or acknowledgements of lesser block numbers are treated as retransmissions
			switch session.mostRecentMessage.(type) {
			case readMessage:
				// pass
			case acknowledgeMessage:
				if session.mostRecentMessage.(acknowledgeMessage).blockNumber == session.blockNumber {
					awaitingRequest = false
				} else if session.mostRecentMessage.(acknowledgeMessage).blockNumber > session.blockNumber {
					return errors.New("Out of sync blockNumber")
				}
			case errorMessage:
				return errors.New(session.mostRecentMessage.(errorMessage).explanation)
			default:
				return errors.New("Client requested invalid operation during established connection")
			}

			i += 1
		}

		session.blockNumber += 1
	}

	return nil
}

func (session *tftpSession) write() error {
	var wroteEverything bool = false
	var unixMicro int64
	var err error

	// we expect that client will acknowledge first data block with 1
	session.blockNumber = 1

	// adjust buffer size because of blockSize
	if len(session.sendBuf) != int(dataPreambleLength+session.blockSize) {
		session.sendBuf = make([]byte, session.blockSize+dataPreambleLength)
	}
	if len(session.receiveBuf) != int(dataPreambleLength+session.blockSize) {
		session.receiveBuf = make([]byte, dataPreambleLength+session.blockSize)
	}

	// get access to a file and associated time
	unixMicro, err = session.prepare()
	if err != nil {
		return err
	}
	defer session.overwriteFailure(unixMicro)

	// if the client sends an dataMessage then we should acknowledge it
	// if the client sends an errorMessage then we log it and return error
	// if the client sends anything else return error
	for !wroteEverything {
		// setup for receive loop
		session.lastValidMessage = session.mostRecentMessage
		var i int = 1
		var awaitingRequest = true

		// read until acknowledgement with correct blockNumber, handle gracefully retransmission
		for awaitingRequest {
			if i > 5 {
				return errors.New("Underlying network may be bad, many retransmiteed messages")
			}

			if err = session.receive(); err != nil {
				return err
			}

			switch session.mostRecentMessage.(type) {
			case writeMessage:
				// pass
			case dataMessage:
				if session.mostRecentMessage.(dataMessage).blockNumber == session.blockNumber {
					awaitingRequest = false
				} else if session.mostRecentMessage.(dataMessage).blockNumber > session.blockNumber {
					return errors.New("Out of sync blockNumber")
				}
			case errorMessage:
				return errors.New(session.mostRecentMessage.(errorMessage).explanation)
			default:
				return errors.New("Client requested invalid operation during established connection")
			}
		}

		// write to file
		err = session.writeFile()
		if errors.Is(err, io.EOF) {
			// It is okay to try to close an already closed file, the second close just fails
			err = session.overwriteSuccess(unixMicro)
			if err != nil {
				return err
			}

			wroteEverything = true
		} else if err != nil {
			return err
		}

		// acknowledge
		if err = session.acknowledgeMessage(); err != nil {
			return err
		}

		session.blockNumber += 1
	}

	return nil
}

func (session *tftpSession) updateOptions(options map[string]string) error {
	for keyCased, valueAscii := range options {
		key := strings.ToLower(keyCased)
		valueInt, err := strconv.ParseInt(valueAscii, 10, 64)
		if err != nil {
			panic(err)
		}

		switch key {
		case "blksize":
			if valueInt < 8 || valueInt > 65464 {
				return errors.New(fmt.Sprintf("Invalid blksize value %v requested by client", valueInt))
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
			if valueInt == 0 {
				/*
				 * tsize of 0 as tsize in read request is special
				 * server responds in optionAcknowledge message with size of file
				 */
				if session.opcode == opcodeReadByte {
					info, err := os.Lstat(session.filename)
					if err != nil {
						return errors.New(fmt.Sprintf("Unable to get the size of %v", session.filename))
					}
					valueInt = info.Size()
				} else {
					return errors.New(fmt.Sprintf("Invalid blksize value %v requested by client", valueInt))
				}
			}

			session.transferSize = uint64(valueInt)
			options["tsize"] = strconv.FormatInt(valueInt, 10)
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
