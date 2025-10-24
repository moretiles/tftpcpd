package main

import (
	"bytes"
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

	buf := make([]byte, 0xffff)
	output := []byte{0, opcodeReadByte, 'f', 'i', 'l', 'e', 0, 'o', 'c', 't', 'a', 'l', 0, 's', 'y', 'n', 't', 'a', 'x', 0, 'o', 'n', 0}
	err := readMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpReadRequest is nil")
	}

	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
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

	buf := make([]byte, 0xffff)
	output := []byte{0, opcodeWriteByte, 'f', 'i', 'l', 'e', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0, 's', 'y', 'n', 't', 'a', 'x', 0, 'o', 'n', 0}
	err := writeMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpWriteRequest is nil")
	}
	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
	}
}

func TestDataMessageAsBytes(t *testing.T) {
	var blockNumber uint16 = 0xabcd
	data := []byte{5, 4, 3, 2, 1}
	message := newDataMessage(blockNumber, data)

	buf := make([]byte, 0xffff)
	output := []byte{0, opcodeDataByte, byte(blockNumber >> 8), byte(blockNumber % 0x100)}
	output = append(output, data...)
	err := dataMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpDta is nil")
	}

	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
	}
}

func TestAcknowledgeMessageAsBytes(t *testing.T) {
	var blockNumber uint16 = 0xabcd
	message := newAcknowledgeMessage(blockNumber)

	buf := make([]byte, 0xffff)
	output := []byte{0, opcodeAcknowledgeByte, byte(blockNumber >> 8), byte(blockNumber % 0x100)}
	err := acknowledgeMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
	}

	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
	}
}

func TestErrorMessageAsBytes(t *testing.T) {
	var errorCode uint16 = errorCodeUndefined
	explanation := "among us"
	message := newErrorMessage(errorCode, explanation)

	buf := make([]byte, 0xffff)
	output := []byte{0, opcodeErrorByte, 0, byte(errorCode)}
	output = append(output, explanation...)
	output = append(output, '\x00')
	err := errorMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
	}

	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
	}
}

func TestOptionAcknowledgeMessageAsBytes(t *testing.T) {
	options := make(map[string]string)
	key1 := "syntax"
	val1 := "on"
	options[key1] = val1
	buf := make([]byte, 0xffff)
	message := newOptionAcknowledgeMessage(options)

	output := []byte{0, opcodeOptionAcknowledgeByte}
	output = append(output, []byte(key1+"\x00")...)
	output = append(output, []byte(val1+"\x00")...)
	err := optionAcknowledgeMessageAsBytes(message, &buf)
	if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
	}

	if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
		return
	}
}
