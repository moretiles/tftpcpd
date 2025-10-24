package main

import (
	"net"
	"strings"
	"time"
)

const (
	logEventNewSession = iota
	logEventEndSession
	logEventErrorSession
)

const (
	opcodeReadByte = 1 + iota
	opcodeWriteByte
	opcodeDataByte
	opcodeAcknowledgeByte
	opcodeErrorByte
	opcodeOptionAcknowledgeByte
)

const (
	errorCodeUndefined = iota
	errorCodeNoSuchFile
	errorCodeAccessViolation
	errorCodeTooMuchData
	errorCodeIllegalOperation
	errorCodeUnknownTransferId
	errorCodeFileAlreadyExists
	errorCodeNoSuchUser
)

type readMessage struct {
	filename string
	mode     string
	options  map[string]string
}

func newReadMessage(filename string, mode string, options map[string]string) readMessage {
	return readMessage{filename, mode, options}
}

type writeMessage struct {
	filename string
	mode     string
	options  map[string]string
}

func newWriteMessage(filename string, mode string, options map[string]string) writeMessage {
	return writeMessage{filename, mode, options}
}

type dataMessage struct {
	blockNumber uint16
	body        []byte
}

func newDataMessage(blockNumber uint16, body []byte) dataMessage {
	return dataMessage{blockNumber, body}
}

type acknowledgeMessage struct {
	blockNumber uint16
}

func newAcknowledgeMessage(blockNumber uint16) acknowledgeMessage {
	return acknowledgeMessage{blockNumber}
}

type errorMessage struct {
	errorCode   uint16
	explanation string
}

func newErrorMessage(errorCode uint16, explanation string) errorMessage {
	return errorMessage{errorCode, explanation}
}

type optionAcknowledgeMessage struct {
	options map[string]string
}

func newOptionAcknowledgeMessage(options map[string]string) optionAcknowledgeMessage {
	return optionAcknowledgeMessage{options}
}

type tftpState struct {
	// set before connection established
	destination net.UDPAddr

	// set when connection established
	opcode   uint16
	filename string
	mode     string
	options  map[string]string

	// values determined by options
	blockSize    uint16
	timeout      time.Duration
	transferSize uint64
	windowSize   uint16

	// updated upon acknowledgements
	blockNumber           uint16
	totalBytesTransferred uint32
}

func newTftpState(destination net.UDPAddr, message string) (tftpState, error) {
	var state tftpState

	state.blockSize = 512
	state.timeout = time.Second * 5
	state.transferSize = 0 // 0 if unknown
	state.windowSize = 512
	state.options = make(map[string]string)

	// handle client iniating read request / write request
	/*
	 * 2 bytes for opcode
	 * 1 byte minimum for filename
	 * 1 byte null terminator for filename
	 * 5 byte minimum for mode ("octet" is 5 bytes, "mail" is obselete according to RFC 1350)
	 * 1 byte null terminator for filename
	 */
	initialRequestMinLength := 2 + 1 + 1 + 5 + 1
	totalMessageLength := len(message)
	currentMessagePosition := 0

	if totalMessageLength < initialRequestMinLength {
		// error
	}
	state.opcode = uint16(message[0])
	currentMessagePosition++
	state.filename = string(message[currentMessagePosition:])
	currentMessagePosition += len(state.filename) + 1
	if currentMessagePosition >= totalMessageLength {
		// error
	}
	state.mode = strings.ToLower(string(message[currentMessagePosition:]))
	currentMessagePosition += len(state.mode) + 1
	for currentMessagePosition < totalMessageLength {
		optionKey := string(message[currentMessagePosition:])
		currentMessagePosition += len(optionKey) + 1

		if currentMessagePosition >= totalMessageLength {
			// error
		}
		optionVal := string(message[currentMessagePosition:])
		currentMessagePosition += len(optionVal) + 1

		state.options[optionKey] = optionVal
	}

	return state, nil
}

func MessageAsBytes(message any, buf *[]byte) error {
	switch message.(type) {
	case readMessage:
		readMessageAsBytes(message.(readMessage), buf)
	case writeMessage:
		writeMessageAsBytes(message.(writeMessage), buf)
	case dataMessage:
		dataMessageAsBytes(message.(dataMessage), buf)
	case acknowledgeMessage:
		acknowledgeMessageAsBytes(message.(acknowledgeMessage), buf)
	case errorMessage:
		errorMessageAsBytes(message.(errorMessage), buf)
	case optionAcknowledgeMessage:
		optionAcknowledgeMessageAsBytes(message.(optionAcknowledgeMessage), buf)
	default:
		// error
	}

	return nil
}

/*
 * Structure of Read Message in byte is:
 * 2 byte opcode = 0x0001
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func readMessageAsBytes(message readMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeReadByte)

	// append filename
	*buf = append(*buf, []byte(message.filename+"\x00")...)

	// append mode
	*buf = append(*buf, []byte(message.mode+"\x00")...)

	// append options (if any)
	for k, v := range message.options {
		*buf = append(*buf, []byte(k+"\x00"+v+"\x00")...)
	}

	return nil
}

/*
 * Structure of Write Message in bytes is:
 * 2 byte opcode = 0x0002
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func writeMessageAsBytes(message writeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeWriteByte)

	// append filename
	*buf = append(*buf, []byte(message.filename+"\x00")...)

	// append mode
	*buf = append(*buf, []byte(message.mode+"\x00")...)

	// append options (if any)
	for k, v := range message.options {
		*buf = append(*buf, []byte(k+"\x00"+v+"\x00")...)
	}

	return nil
}

/*
 * Structure of Data Message in bytes is:
 * 2 byte opcode = 0x0003
 * 2 byte blockNumber
 * [many]char fileDataItself
 */
func dataMessageAsBytes(message dataMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeDataByte)

	// append block number as big endian uint16
	*buf = append(*buf, byte(message.blockNumber>>8), byte(message.blockNumber))

	// append data itself
	*buf = append(*buf, message.body...)

	return nil
}

/*
 * Structure of Acknowledge Message in bytes is:
 * 2 byte opcode = 0x0004
 * 2 byte acknowledgedBlockNumber
 */
func acknowledgeMessageAsBytes(message acknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeAcknowledgeByte)

	// append block number as big endian uint16
	*buf = append(*buf, byte(message.blockNumber>>8), byte(message.blockNumber))

	return nil
}

/*
 * Structure of Error Message in bytes is:
 * 2 byte opcode = 0x0005
 * 2 byte errorCode
 * string (null terminated sequence of bytes) humanReadableErrorMessage
 */
func errorMessageAsBytes(message errorMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeErrorByte)

	// append error code as big endian uint16
	*buf = append(*buf, byte(message.errorCode>>8), byte(message.errorCode))

	// append human readable explanation
	*buf = append(*buf, message.explanation...)
	*buf = append(*buf, '\x00')

	// send as byte[]
	return nil
}

/*
 * Structure of Option Acknowledgement Message in bytes is:
 * 2 byte opcode = 0x0006
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func optionAcknowledgeMessageAsBytes(message optionAcknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	//append opcode
	*buf = append(*buf, 0, opcodeOptionAcknowledgeByte)

	// append options (if any)
	for k, v := range message.options {
		*buf = append(*buf, []byte(k+"\x00"+v+"\x00")...)
	}

	// send as byte[]
	return nil
}
