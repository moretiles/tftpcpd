package main

import (
    "testing"
    "bytes"
)

/*
type tftpState struct (
    // set when connection established
    opcode uint16
    filename string
    mode uint16
    options map[string][string]

    // values determined by options
    blockSize uint16
    timeout uint16
    transferSize uint16
    windowSize uint16

    // updated upon acknowledgements
    blockNumber uint16
    totalBytesTransferred uint32
)

func tftpSend_test(address UDPAddr, opcode [2]byte, state tftpState, buf []byte) error {

    // set slice length to 0 while retaining capacity
    buf = buf[:0]

    switch opcode {
    case bytes.Equal(opcode, opcodeReadRequest):
        tftpReadRequest(state.filename, state.mode, state.options, buf)
    case bytes.Equal(opcode, opcodeWriteRequest):
        tftpWriteRequest(state.filename, state.mode, state.options, buf)
    default:
        // error
    }

    udp.send(buf)
    return nil
}
*/

func TestTftpReadRequest(t *testing.T) {
	var state tftpState
	state.filename = "file"
	state.mode = "octal"
	state.options = make(map[string]string)
    key1 := "syntax"
    val1 := "on"
    state.options[key1] = val1
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeReadRequestByte, 'f', 'i', 'l', 'e', 0, 'o', 'c', 't', 'a', 'l', 0}
    err := tftpReadRequest(state.filename, state.mode, state.options, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpReadRequest is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}

func TestTftpWriteRequest(t *testing.T) {
	var state tftpState
	state.filename = "file"
	state.mode = "netascii"
	state.options = make(map[string]string)
    key1 := "syntax"
    val1 := "on"
    state.options[key1] = val1
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeWriteRequestByte, 'f', 'i', 'l', 'e', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0}
    err := tftpWriteRequest(state.filename, state.mode, state.options, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpWriteRequest is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}

func TestTftpData(t *testing.T) {
	var state tftpState
	state.blockNumber = 0xabcd
    data := []byte{5, 4, 3, 2, 1}
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeDataByte, byte(state.blockNumber >> 8), byte(state.blockNumber % 0x100)}
    output = append(output, data...)
    err := tftpData(state.blockNumber, data, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpDta is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}

func TestTftpAcknowledge(t *testing.T) {
	var state tftpState
	state.blockNumber = 0xabcd
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeAcknowledgeByte, byte(state.blockNumber >> 8), byte(state.blockNumber % 0x100)}
    err := tftpAcknowledge(state.blockNumber, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}

func TestTftpError(t *testing.T) {
    errorMessage := []byte("among us")
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeErrorByte, 0, errorCodeUndefined}
    output = append(output, errorMessage...)
    output = append(output, '\x00')
    err := tftpError(errorCodeUndefined, errorMessage, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}

func TestTftpOptionAcknowledge(t *testing.T) {
	var state tftpState
	state.options = make(map[string]string)
    key1 := "syntax"
    val1 := "on"
    state.options[key1] = val1
	buf := make([]byte, 0xffff)
    // zero out length, keep capacity
    buf = buf[:0]

	output := []byte{0, opcodeOptionAcknowledgeByte}
    output = append(output, []byte(key1 + "\x00")...)
    output = append(output, []byte(val1 + "\x00")...)
    err := tftpOptionAcknowledge(state.options, buf)
    if err != nil {
		t.Errorf("Some argument passed to tftpAcknowledge is nil")
    }
    buf = buf[:len(output)]

    if !bytes.Equal(output, buf) {
		t.Errorf("%v != %v\n", output, buf)
        return
    }
}
