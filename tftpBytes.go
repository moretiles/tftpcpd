package main

import (
    "net"
	"time"
	"strings"
)

const (
    logEventNewSession = iota
    logEventEndSession 
    logEventErrorSession 
)

const (
    opcodeReadRequestByte = 1 + iota
    opcodeWriteRequestByte
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

type tftpState struct {
	// set before connection established
	destination net.UDPAddr

    // set when connection established
    opcode uint16
    filename string
    mode string
    options map[string]string

    // values determined by options
    blockSize uint16
    timeout time.Duration
    transferSize uint64
    windowSize uint16

    // updated upon acknowledgements
    blockNumber uint16
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

    if(totalMessageLength < initialRequestMinLength){
        // error
    }
    state.opcode = uint16(message[0])
    currentMessagePosition++
    state.filename = string(message[currentMessagePosition:])
    currentMessagePosition += len(state.filename) + 1
    if(currentMessagePosition >= totalMessageLength){
        // error
    }
    state.mode = strings.ToLower(string(message[currentMessagePosition:]))
    currentMessagePosition += len(state.mode) + 1
    for currentMessagePosition < totalMessageLength {
        optionKey := string(message[currentMessagePosition:])
        currentMessagePosition += len(optionKey) + 1

        if(currentMessagePosition >= totalMessageLength){
            // error
        }
        optionVal := string(message[currentMessagePosition:])
        currentMessagePosition += len(optionVal) + 1

        state.options[optionKey] = optionVal
    }

    return state, nil
}

func tftpSend(state tftpState, opcode uint16, errorCode uint16, data []byte, buf []byte) error {
    // set slice length to 0 while retaining capacity
    buf = buf[:0]

    switch opcode {
    case opcodeReadRequestByte:
        tftpReadRequest(state.filename, state.mode, state.options, buf)
    case opcodeWriteRequestByte:
        tftpWriteRequest(state.filename, state.mode, state.options, buf)
    case opcodeDataByte:
        tftpData(state.blockNumber, data, buf)
    case opcodeAcknowledgeByte:
        tftpAcknowledge(state.blockNumber, buf)
    case opcodeErrorByte:
        tftpError(errorCode, data, buf)
    case opcodeOptionAcknowledgeByte:
        tftpOptionAcknowledge(state.options, buf)
    default:
        // error
    }

    //udp.send(buf)
    return nil
}

/*
 * Structure of Read Request is:
 * 2 byte opcode = 0x0001
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func tftpReadRequest(filename string, mode string, options map[string]string, buf []byte) error {
	// append opcode
    buf = append(buf, 0, opcodeReadRequestByte)

	// append filename
    buf = append(buf, []byte(filename + "\x00")...)

	// append mode
	buf = append(buf, []byte(mode + "\x00")...)

	// append options (if any)
    for k, v := range options {
        buf = append(buf, []byte(k + "\x00" + v + "\x00")...)
    }

    return nil
}

/*
 * Structure of Read Request is:
 * 2 byte opcode = 0x0002
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func tftpWriteRequest(filename string, mode string, options map[string]string, buf []byte) error {
	// append opcode
    buf = append(buf, 0, opcodeWriteRequestByte)

	// append filename
    buf = append(buf, []byte(filename + "\x00")...)

	// append mode
	buf = append(buf, []byte(mode + "\x00")...)

	// append options (if any)
    for k, v := range options {
        buf = append(buf, []byte(k + "\x00" + v + "\x00")...)
    }

    return nil
}

/*
 * Structure of Data is:
 * 2 byte opcode = 0x0003
 * 2 byte blockNumber
 * [many]char fileDataItself
 */
func tftpData(blockNumber uint16, data []byte, buf []byte) error {
	// append opcode
    buf = append(buf, 0, opcodeDataByte)

	// append block number as big endian uint16
	buf = append(buf, byte(blockNumber >> 8), byte(blockNumber))

	// append data itself
	buf = append(buf, data...)

    return nil
}

/*
 * Structure of Acknowledge is:
 * 2 byte opcode = 0x0004
 * 2 byte acknowledgedBlockNumber
 */
func tftpAcknowledge(blockNumber uint16, buf []byte) error {
    // append opcode
    buf = append(buf, 0, opcodeAcknowledgeByte)

	// append block number as big endian uint16
	buf = append(buf, byte(blockNumber >> 8), byte(blockNumber))

    return nil
}

/*
 * Structure of Error is:
 * 2 byte opcode = 0x0005
 * 2 byte errorCode
 * string (null terminated sequence of bytes) humanReadableErrorMessage
 */
func tftpError(errorCode uint16, explanation []byte, buf []byte) error {
	// append opcode
    buf = append(buf, 0, opcodeErrorByte)

	// append error code as big endian uint16
	buf = append(buf, byte(errorCode >> 8), byte(errorCode))

	// append human readable explanation
    buf = append(buf, explanation...)
    buf = append(buf, '\x00')

    // send as byte[]
    return nil
}

/*
 * Structure of Error is:
 * 2 byte opcode = 0x0006
 * [many](key string, value string) optionsAsKeyValuePairs
 */
func tftpOptionAcknowledge(options map[string]string, buf []byte) error {
	//append opcode
    buf = append(buf, 0, opcodeOptionAcknowledgeByte)

	// append options (if any)
    for k, v := range options {
        buf = append(buf, []byte(k + "\x00" + v + "\x00")...)
    }

    // send as byte[]
    return nil
}
