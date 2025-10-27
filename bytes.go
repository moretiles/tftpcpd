package main

import (
	"errors"
	"io"
	"slices"
	"strings"
)

var ErrUnknownOpcode = errors.New("Unknown opcode found when decoding message")
var ErrUnknownMessage = errors.New("Unknown message type used to encode message")
var ErrShortMessage = errors.New("Impossibly short message received")
var ErrUnterminatedNullString = errors.New("Null byte not found")

const (
	opcodeInvalid = iota
	opcodeReadByte
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
	errorCodeOptionAcknowledgeSurprise
)

/*
 * Structure of Read Message in byte is:
 * 2 byte opcode = 0x0001
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type readMessage struct {
	filename string
	mode     string
	options  map[string]string
}

func newReadMessage(filename string, mode string, options map[string]string) readMessage {
	return readMessage{filename, mode, options}
}

/*
 * Structure of Write Message in bytes is:
 * 2 byte opcode = 0x0002
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type writeMessage struct {
	filename string
	mode     string
	options  map[string]string
}

func newWriteMessage(filename string, mode string, options map[string]string) writeMessage {
	return writeMessage{filename, mode, options}
}

/*
 * Structure of Data Message in bytes is:
 * 2 byte opcode = 0x0003
 * 2 byte blockNumber
 * [many]char fileDataItself
 */
type dataMessage struct {
	blockNumber uint16
	body        []byte
}

func newDataMessage(blockNumber uint16, body []byte) dataMessage {
	return dataMessage{blockNumber, body}
}

/*
 * Structure of Acknowledge Message in bytes is:
 * 2 byte opcode = 0x0004
 * 2 byte acknowledgedBlockNumber
 */
type acknowledgeMessage struct {
	blockNumber uint16
}

func newAcknowledgeMessage(blockNumber uint16) acknowledgeMessage {
	return acknowledgeMessage{blockNumber}
}

/*
 * Structure of Error Message in bytes is:
 * 2 byte opcode = 0x0005
 * 2 byte errorCode
 * string (null terminated sequence of bytes) humanReadableErrorMessage
 */
type errorMessage struct {
	errorCode   uint16
	explanation string
}

func newErrorMessage(errorCode uint16, explanation string) errorMessage {
	return errorMessage{errorCode, explanation}
}

/*
 * Structure of Option Acknowledgement Message in bytes is:
 * 2 byte opcode = 0x0006
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type optionAcknowledgeMessage struct {
	options map[string]string
}

func newOptionAcknowledgeMessage(options map[string]string) optionAcknowledgeMessage {
	return optionAcknowledgeMessage{options}
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
		return ErrUnknownMessage
	}

	return nil
}

func popNullString(buf *[]byte) (string, error) {
	nullBytePos := slices.Index(*buf, 0)
	if nullBytePos < 0 || nullBytePos >= len(*buf) {
		return "", ErrUnterminatedNullString
	}
	nullString := string((*buf)[:nullBytePos])
	if nullBytePos+1 >= len(*buf) {
		*buf = (*buf)[:0]
		return nullString, io.EOF
	}
	*buf = (*buf)[nullBytePos+1:]

	return nullString, nil
}

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
		*buf = append(*buf, []byte(strings.ToLower(k)+"\x00"+v+"\x00")...)
	}

	return nil
}

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
		*buf = append(*buf, []byte(strings.ToLower(k)+"\x00"+v+"\x00")...)
	}

	return nil
}

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

func acknowledgeMessageAsBytes(message acknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, opcodeAcknowledgeByte)

	// append block number as big endian uint16
	*buf = append(*buf, byte(message.blockNumber>>8), byte(message.blockNumber))

	return nil
}

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

	return nil
}

func optionAcknowledgeMessageAsBytes(message optionAcknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	//append opcode
	*buf = append(*buf, 0, opcodeOptionAcknowledgeByte)

	// append options (if any)
	for k, v := range message.options {
		*buf = append(*buf, []byte(strings.ToLower(k)+"\x00"+v+"\x00")...)
	}

	return nil
}

func BytesAsMessage(buf []byte) (any, error) {
	var message any
	var opcode byte
	var err error

	if len(buf) < 2 {
		return nil, ErrShortMessage
	}

	// leading byte of opcode is always 0x00
	if buf[0] != 0 {
		return nil, ErrUnknownOpcode
	}

	// 2nd byte of opcode is what determines message types
	opcode = buf[1]
	buf = buf[2:]
	switch opcode {
	case opcodeReadByte:
		message, err = bytesAsReadMessage(buf)
		if err != nil {
			return nil, err
		}
	case opcodeWriteByte:
		message, err = bytesAsWriteMessage(buf)
		if err != nil {
			return nil, err
		}
	case opcodeDataByte:
		message, err = bytesAsDataMessage(buf)
		if err != nil {
			return nil, err
		}
	case opcodeAcknowledgeByte:
		message, err = bytesAsAcknowledgeMessage(buf)
		if err != nil {
			return nil, err
		}
	case opcodeErrorByte:
		message, err = bytesAsErrorMessage(buf)
		if err != nil {
			return nil, err
		}
	case opcodeOptionAcknowledgeByte:
		message, err = bytesAsOptionAcknowledgeMessage(buf)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnknownOpcode
	}

	return message, nil
}

func bytesAsReadMessage(buf []byte) (readMessage, error) {
	var filename string
	var mode string
	var options map[string]string = make(map[string]string)

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return readMessage{}, ErrShortMessage
	}

	filename, err := popNullString(&buf)
	if err != nil {
		return readMessage{}, ErrUnterminatedNullString
	}

	mode, err = popNullString(&buf)
	if err != nil {
		return readMessage{}, ErrUnterminatedNullString
	}
	mode = strings.ToLower(mode)

	for len(buf) > 0 {
		key, err := popNullString(&buf)
		if err != nil {
			return readMessage{}, ErrUnterminatedNullString
		}
		key = strings.ToLower(key)

		val, err := popNullString(&buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return readMessage{}, ErrUnterminatedNullString
		}

		options[key] = val
	}

	return newReadMessage(filename, mode, options), nil
}

func bytesAsWriteMessage(buf []byte) (writeMessage, error) {
	var filename string
	var mode string
	var options map[string]string = make(map[string]string)

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return writeMessage{}, ErrShortMessage
	}

	filename, err := popNullString(&buf)
	if err != nil {
		return writeMessage{}, ErrUnterminatedNullString
	}

	mode, err = popNullString(&buf)
	if err != nil {
		return writeMessage{}, ErrUnterminatedNullString
	}
	mode = strings.ToLower(mode)

	for len(buf) > 0 {
		key, err := popNullString(&buf)
		if err != nil {
			return writeMessage{}, ErrUnterminatedNullString
		}
		key = strings.ToLower(key)

		val, err := popNullString(&buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return writeMessage{}, ErrUnterminatedNullString
		}

		options[key] = val
	}

	return newWriteMessage(filename, mode, options), nil
}

func bytesAsDataMessage(buf []byte) (dataMessage, error) {
	var blockNumber uint16
	var data []byte

	minPossibleLen := 2

	if len(buf) < minPossibleLen {
		return dataMessage{}, ErrShortMessage
	}

	blockNumber += uint16(buf[0]) << 8
	blockNumber += uint16(buf[1])

	if len(buf) >= 2 {
		data = buf[2:]
	}

	return newDataMessage(blockNumber, data), nil
}

func bytesAsAcknowledgeMessage(buf []byte) (acknowledgeMessage, error) {
	var blockNumber uint16

	minPossibleLen := 2

	if len(buf) < minPossibleLen {
		return acknowledgeMessage{}, ErrShortMessage
	}

	blockNumber += uint16(buf[0]) << 8
	blockNumber += uint16(buf[1])

	return newAcknowledgeMessage(blockNumber), nil
}

func bytesAsErrorMessage(buf []byte) (errorMessage, error) {
	var errorCode uint16
	var explanation string

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return errorMessage{}, ErrShortMessage
	}

	errorCode += uint16(buf[0]) << 8
	errorCode += uint16(buf[1])

	buf = buf[2:]
	explanation, err := popNullString(&buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return errorMessage{}, ErrUnterminatedNullString
	}

	return newErrorMessage(errorCode, explanation), nil
}

func bytesAsOptionAcknowledgeMessage(buf []byte) (optionAcknowledgeMessage, error) {
	var options map[string]string = make(map[string]string)

	minPossibleLen := 1 + 1 + 1 + 1

	if len(buf) < minPossibleLen {
		return optionAcknowledgeMessage{}, ErrShortMessage
	}

	for len(buf) > 0 {
		key, err := popNullString(&buf)
		if err != nil {
			return optionAcknowledgeMessage{}, ErrUnterminatedNullString
		}
		key = strings.ToLower(key)

		val, err := popNullString(&buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return optionAcknowledgeMessage{}, ErrUnterminatedNullString
		}

		options[key] = val
	}

	return newOptionAcknowledgeMessage(options), nil
}
