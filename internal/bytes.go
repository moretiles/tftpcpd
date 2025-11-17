package internal

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
	OpcodeInvalid = iota
	OpcodeReadByte
	OpcodeWriteByte
	OpcodeDataByte
	OpcodeAcknowledgeByte
	OpcodeErrorByte
	OpcodeOptionAcknowledgeByte
)

const (
	ErrorCodeUndefined = iota
	ErrorCodeNoSuchFile
	ErrorCodeAccessViolation
	ErrorCodeTooMuchData
	ErrorCodeIllegalOperation
	ErrorCodeUnknownTransferId
	ErrorCodeFileAlreadyExists
	ErrorCodeNoSuchUser
	ErrorCodeOptionAcknowledgeSurprise
)

/*
 * Structure of Read Message in byte is:
 * 2 byte opcode = 0x0001
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type ReadMessage struct {
	Filename string
	Mode     string
	Options  map[string]string
}

func NewReadMessage(filename string, mode string, options map[string]string) ReadMessage {
	return ReadMessage{filename, mode, options}
}

/*
 * Structure of Write Message in bytes is:
 * 2 byte opcode = 0x0002
 * string (null terminated sequence of bytes) filename
 * string (null terminated sequence of bytes) mode
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type WriteMessage struct {
	Filename string
	Mode     string
	Options  map[string]string
}

func NewWriteMessage(filename string, mode string, options map[string]string) WriteMessage {
	return WriteMessage{filename, mode, options}
}

/*
 * Structure of Data Message in bytes is:
 * 2 byte opcode = 0x0003
 * 2 byte blockNumber
 * [many]char fileDataItself
 */
type DataMessage struct {
	BlockNumber uint16
	Body        []byte
}

func NewDataMessage(blockNumber uint16, body []byte) DataMessage {
	return DataMessage{blockNumber, body}
}

/*
 * Structure of Acknowledge Message in bytes is:
 * 2 byte opcode = 0x0004
 * 2 byte acknowledgedBlockNumber
 */
type AcknowledgeMessage struct {
	BlockNumber uint16
}

func NewAcknowledgeMessage(blockNumber uint16) AcknowledgeMessage {
	return AcknowledgeMessage{blockNumber}
}

/*
 * Structure of Error Message in bytes is:
 * 2 byte opcode = 0x0005
 * 2 byte errorCode
 * string (null terminated sequence of bytes) humanReadableErrorMessage
 */
type ErrorMessage struct {
	ErrorCode   uint16
	Explanation string
}

func NewErrorMessage(errorCode uint16, explanation string) ErrorMessage {
	return ErrorMessage{errorCode, explanation}
}

/*
 * Structure of Option Acknowledgement Message in bytes is:
 * 2 byte opcode = 0x0006
 * [many](key string, value string) optionsAsKeyValuePairs
 */
type OptionAcknowledgeMessage struct {
	Options map[string]string
}

func NewOptionAcknowledgeMessage(options map[string]string) OptionAcknowledgeMessage {
	return OptionAcknowledgeMessage{options}
}

func MessageAsBytes(message any, buf *[]byte) error {
	switch message.(type) {
	case ReadMessage:
		readMessageAsBytes(message.(ReadMessage), buf)
	case WriteMessage:
		writeMessageAsBytes(message.(WriteMessage), buf)
	case DataMessage:
		dataMessageAsBytes(message.(DataMessage), buf)
	case AcknowledgeMessage:
		acknowledgeMessageAsBytes(message.(AcknowledgeMessage), buf)
	case ErrorMessage:
		errorMessageAsBytes(message.(ErrorMessage), buf)
	case OptionAcknowledgeMessage:
		optionAcknowledgeMessageAsBytes(message.(OptionAcknowledgeMessage), buf)
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

func readMessageAsBytes(message ReadMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, OpcodeReadByte)

	// append filename
	*buf = append(*buf, []byte(message.Filename+"\x00")...)

	// append mode
	*buf = append(*buf, []byte(message.Mode+"\x00")...)

	// append options (if any)
	for k, v := range message.Options {
		*buf = append(*buf, []byte(strings.ToLower(k)+"\x00"+v+"\x00")...)
	}

	return nil
}

func writeMessageAsBytes(message WriteMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, OpcodeWriteByte)

	// append filename
	*buf = append(*buf, []byte(message.Filename+"\x00")...)

	// append mode
	*buf = append(*buf, []byte(message.Mode+"\x00")...)

	// append options (if any)
	for k, v := range message.Options {
		*buf = append(*buf, []byte(strings.ToLower(k)+"\x00"+v+"\x00")...)
	}

	return nil
}

func dataMessageAsBytes(message DataMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, OpcodeDataByte)

	// append block number as big endian uint16
	*buf = append(*buf, byte(message.BlockNumber>>8), byte(message.BlockNumber))

	// append data itself
	*buf = append(*buf, message.Body...)

	return nil
}

func acknowledgeMessageAsBytes(message AcknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, OpcodeAcknowledgeByte)

	// append block number as big endian uint16
	*buf = append(*buf, byte(message.BlockNumber>>8), byte(message.BlockNumber))

	return nil
}

func errorMessageAsBytes(message ErrorMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	// append opcode
	*buf = append(*buf, 0, OpcodeErrorByte)

	// append error code as big endian uint16
	*buf = append(*buf, byte(message.ErrorCode>>8), byte(message.ErrorCode))

	// append human readable explanation
	*buf = append(*buf, message.Explanation...)
	*buf = append(*buf, '\x00')

	return nil
}

func optionAcknowledgeMessageAsBytes(message OptionAcknowledgeMessage, buf *[]byte) error {
	// zero out length, keep capacity
	*buf = (*buf)[:0]

	//append opcode
	*buf = append(*buf, 0, OpcodeOptionAcknowledgeByte)

	// append options (if any)
	for k, v := range message.Options {
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
	case OpcodeReadByte:
		message, err = bytesAsReadMessage(buf)
		if err != nil {
			return nil, err
		}
	case OpcodeWriteByte:
		message, err = bytesAsWriteMessage(buf)
		if err != nil {
			return nil, err
		}
	case OpcodeDataByte:
		message, err = bytesAsDataMessage(buf)
		if err != nil {
			return nil, err
		}
	case OpcodeAcknowledgeByte:
		message, err = bytesAsAcknowledgeMessage(buf)
		if err != nil {
			return nil, err
		}
	case OpcodeErrorByte:
		message, err = bytesAsErrorMessage(buf)
		if err != nil {
			return nil, err
		}
	case OpcodeOptionAcknowledgeByte:
		message, err = bytesAsOptionAcknowledgeMessage(buf)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnknownOpcode
	}

	return message, nil
}

func bytesAsReadMessage(buf []byte) (ReadMessage, error) {
	var filename string
	var mode string
	var options map[string]string

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return ReadMessage{}, ErrShortMessage
	}

	filename, err := popNullString(&buf)
	if err != nil {
		return ReadMessage{}, errors.Join(err, ErrUnterminatedNullString)
	}

	mode, err = popNullString(&buf)
	mode = strings.ToLower(mode)
	// Only process options if they exist
	if errors.Is(err, io.EOF) {
		return NewReadMessage(filename, mode, options), nil
	} else if err != nil {
		return ReadMessage{}, errors.Join(err, ErrUnterminatedNullString)
	}

	options, err = bytesAsOptionMap(buf)
	if err != nil {
		return ReadMessage{}, err
	}
	return NewReadMessage(filename, mode, options), nil
}

func bytesAsWriteMessage(buf []byte) (WriteMessage, error) {
	var filename string
	var mode string
	var options map[string]string

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return WriteMessage{}, ErrShortMessage
	}

	filename, err := popNullString(&buf)
	if err != nil {
		return WriteMessage{}, errors.Join(err, ErrUnterminatedNullString)
	}

	mode, err = popNullString(&buf)
	mode = strings.ToLower(mode)
	if errors.Is(err, io.EOF) {
		return NewWriteMessage(filename, mode, options), nil
	} else if err != nil {
		return WriteMessage{}, errors.Join(err, ErrUnterminatedNullString)
	}

	options, err = bytesAsOptionMap(buf)
	if err != nil {
		return WriteMessage{}, err
	}
	return NewWriteMessage(filename, mode, options), nil
}

func bytesAsDataMessage(buf []byte) (DataMessage, error) {
	var blockNumber uint16
	var data []byte

	minPossibleLen := 2

	if len(buf) < minPossibleLen {
		return DataMessage{}, ErrShortMessage
	}

	blockNumber += uint16(buf[0]) << 8
	blockNumber += uint16(buf[1])

	if len(buf) >= 2 {
		data = buf[2:]
	}

	return NewDataMessage(blockNumber, data), nil
}

func bytesAsAcknowledgeMessage(buf []byte) (AcknowledgeMessage, error) {
	var blockNumber uint16

	minPossibleLen := 2

	if len(buf) < minPossibleLen {
		return AcknowledgeMessage{}, ErrShortMessage
	}

	blockNumber += uint16(buf[0]) << 8
	blockNumber += uint16(buf[1])

	return NewAcknowledgeMessage(blockNumber), nil
}

func bytesAsErrorMessage(buf []byte) (ErrorMessage, error) {
	var errorCode uint16
	var explanation string

	minPossibleLen := 1 + 1 + 5 + 1

	if len(buf) < minPossibleLen {
		return ErrorMessage{}, ErrShortMessage
	}

	errorCode += uint16(buf[0]) << 8
	errorCode += uint16(buf[1])

	buf = buf[2:]
	explanation, err := popNullString(&buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return ErrorMessage{}, ErrUnterminatedNullString
	}

	return NewErrorMessage(errorCode, explanation), nil
}

func bytesAsOptionAcknowledgeMessage(buf []byte) (OptionAcknowledgeMessage, error) {
	var options map[string]string

	minPossibleLen := 1 + 1 + 1 + 1

	if len(buf) < minPossibleLen {
		return OptionAcknowledgeMessage{}, ErrShortMessage
	}

	options, err := bytesAsOptionMap(buf)
	if err != nil {
		return OptionAcknowledgeMessage{}, err
	}

	return NewOptionAcknowledgeMessage(options), nil
}

func bytesAsOptionMap(buf []byte) (map[string]string, error) {
	var options map[string]string = make(map[string]string)

	for len(buf) > 0 {
		key, err := popNullString(&buf)
		if err != nil {
			return nil, errors.Join(err, ErrUnterminatedNullString)
		}
		key = strings.ToLower(key)

		val, err := popNullString(&buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, errors.Join(err, ErrUnterminatedNullString)
		}

		options[key] = val
	}

	return options, nil
}
