package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReadMessageAsBytes(t *testing.T) {
	filename := "file"
	mode := "octal"
	options := make(map[string]string)
	key1 := "syntax"
	val1 := "on"
	options[key1] = val1
	message := newReadMessage(filename, mode, options)

	buf := make([]byte, 0, 0xffff)
	output := []byte{0, opcodeReadByte, 'f', 'i', 'l', 'e', 0, 'o', 'c', 't', 'a', 'l', 0, 's', 'y', 'n', 't', 'a', 'x', 0, 'o', 'n', 0}

	err := readMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpReadRequest is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestWriteMessageAsBytes(t *testing.T) {
	filename := "file"
	mode := "netascii"
	options := make(map[string]string)
	key1 := "syntax"
	val1 := "on"
	options[key1] = val1
	message := newWriteMessage(filename, mode, options)

	buf := make([]byte, 0, 0xffff)
	output := []byte{0, opcodeWriteByte, 'f', 'i', 'l', 'e', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0, 's', 'y', 'n', 't', 'a', 'x', 0, 'o', 'n', 0}
	err := writeMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpWriteRequest is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestDataMessageAsBytes(t *testing.T) {
	var blockNumber uint16 = 0xabcd
	data := []byte{5, 4, 3, 2, 1}
	message := newDataMessage(blockNumber, data)

	buf := make([]byte, 0, 0xffff)
	output := []byte{0, opcodeDataByte, byte(blockNumber >> 8), byte(blockNumber % 0x100)}
	output = append(output, data...)
	err := dataMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpDta is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestAcknowledgeMessageAsBytes(t *testing.T) {
	var blockNumber uint16 = 0xabcd
	message := newAcknowledgeMessage(blockNumber)

	buf := make([]byte, 0, 0xffff)
	output := []byte{0, opcodeAcknowledgeByte, byte(blockNumber >> 8), byte(blockNumber % 0x100)}
	err := acknowledgeMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpAcknowledge is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestErrorMessageAsBytes(t *testing.T) {
	var errorCode uint16 = errorCodeUndefined
	explanation := "among us"
	message := newErrorMessage(errorCode, explanation)

	buf := make([]byte, 0, 0xffff)
	output := []byte{0, opcodeErrorByte, 0, byte(errorCode)}
	output = append(output, explanation...)
	output = append(output, '\x00')
	err := errorMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpAcknowledge is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestOptionAcknowledgeMessageAsBytes(t *testing.T) {
	options := make(map[string]string)
	key1 := "syntax"
	val1 := "on"
	options[key1] = val1
	buf := make([]byte, 0, 0xffff)
	message := newOptionAcknowledgeMessage(options)

	output := []byte{0, opcodeOptionAcknowledgeByte}
	output = append(output, []byte(key1+"\x00")...)
	output = append(output, []byte(val1+"\x00")...)
	err := optionAcknowledgeMessageAsBytes(message, &buf)
	if err != nil {
		t.Fatalf("Some argument passed to tftpAcknowledge is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Fatalf("%v != %v\n", output, buf)
	}
}

func TestBytesAsReadMessage(t *testing.T) {
	filename := "vals.zip"
	mode := "octal"
	options := make(map[string]string)
	key1 := "backup"
	val1 := "true"
	options[key1] = val1
	buf := make([]byte, 0, 0xffff)
	buf = append(buf, []byte(filename)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(mode)...)
	buf = append(buf, 0)
	for k, v := range options {
		buf = append(buf, []byte(k)...)
		buf = append(buf, 0)
		buf = append(buf, []byte(v)...)
		buf = append(buf, 0)
	}
	expected := newReadMessage(filename, mode, options)

	message, err := bytesAsReadMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsReadMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected readMessage{%v, %v, %v} != message readMessage{%v, %v, %v}\n", expected.filename, expected.mode, expected.options, message.filename, message.mode, message.options)
	}
}

func TestBytesAsWriteMessage(t *testing.T) {
	filename := "vals.zip"
	mode := "octal"
	options := make(map[string]string)
	key1 := "backup"
	val1 := "true"
	options[key1] = val1
	buf := make([]byte, 0, 0xffff)
	buf = append(buf, []byte(filename)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(mode)...)
	buf = append(buf, 0)
	for k, v := range options {
		buf = append(buf, []byte(k)...)
		buf = append(buf, 0)
		buf = append(buf, []byte(v)...)
		buf = append(buf, 0)
	}
	expected := newWriteMessage(filename, mode, options)

	message, err := bytesAsWriteMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsWriteMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected writeMessage{%v, %v, %v} != message writeMessage{%v, %v, %v}\n", expected.filename, expected.mode, expected.options, message.filename, message.mode, message.options)
	}
}

func TestBytesAsDataMessage(t *testing.T) {
	var blockNumber uint16 = 0x5678
	data := []byte{9, 8, 7, 6, 5, 15, 14, 13, 12, 11}
	buf := make([]byte, 0, 0xffff)
	buf = append(buf, byte(blockNumber>>8), byte(blockNumber))
	buf = append(buf, data...)
	expected := newDataMessage(blockNumber, data)

	message, err := bytesAsDataMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsDataMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected dataMessage{%v, %v} != message dataMessage{%v, %v}\n", expected.blockNumber, expected.body, message.blockNumber, message.body)
	}
}

func TestBytesAsAcknowledgeMessage(t *testing.T) {
	var blockNumber uint16 = 0x5678
	buf := make([]byte, 0, 0xffff)
	buf = append(buf, byte(blockNumber>>8), byte(blockNumber))
	expected := newAcknowledgeMessage(blockNumber)

	message, err := bytesAsAcknowledgeMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsAcknowledgeMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected acknowledgeMessage{%v} != message acknowledgeMessage{%v}\n", expected.blockNumber, message.blockNumber)
	}
}

func TestBytesAsErrorMessage(t *testing.T) {
	var errorCode uint16 = errorCodeTooMuchData
	explanation := "You asked for too much data"
	buf := make([]byte, 0, 0xffff)
	buf = append(buf, byte(errorCode>>8), byte(errorCode))
	buf = append(buf, []byte(explanation)...)
	buf = append(buf, 0)
	expected := newErrorMessage(errorCode, explanation)

	message, err := bytesAsErrorMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsErrorMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected errorMessage{%v, %v} != message errorMessage{%v, %v}\n", expected.errorCode, expected.explanation, message.errorCode, message.explanation)
	}
}

func TestBytesAsOptionAcknowledgeMessage(t *testing.T) {
	options := make(map[string]string)
	key1 := "backup"
	val1 := "true"
	options[key1] = val1
	buf := make([]byte, 0, 0xffff)
	for k, v := range options {
		buf = append(buf, []byte(k)...)
		buf = append(buf, 0)
		buf = append(buf, []byte(v)...)
		buf = append(buf, 0)
	}
	expected := newOptionAcknowledgeMessage(options)

	message, err := bytesAsOptionAcknowledgeMessage(buf)
	if err != nil {
		t.Fatalf("Some error occured when bytesAsOptionAcknowledgeMessage tried parsing buf\n")
	}
	if !reflect.DeepEqual(expected, message) {
		t.Fatalf("expected optionAcknowledgeMessage{%v} != message optionAcknowlegeMessage{%v}\n", expected.options, message.options)
	}
}
