package internal

import (
	"context"
	"database/sql"
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
	DataPreambleLength = 4
)

const (
	ReadAsClient = iota
	WriteAsClient
	ReadAsServer
	WriteAsServer
)

type TftpSession struct {
	// context
	Ctx context.Context

	// used for connection
	DestinationAddr *net.UDPAddr
	Destination     *net.UDPConn
	SendBuf         []byte
	ReceiveBuf      []byte

	// set when opening file
	File      *os.File
	UnixMicro int64

	// set when connection established
	Operation uint16
	Filename  string
	Mode      string
	Options   map[string]string

	// set when negotiating options
	BlockSize    uint16
	Timeout      time.Duration
	TransferSize uint64
	WindowSize   uint16

	// updated upon acknowledgements
	BlockNumber           uint16
	TotalBytesTransferred uint32
	LastValidMessage      any
	MostRecentMessage     any
}

func NewTftpSession(ctx context.Context, destination *net.UDPAddr) (TftpSession, error) {
	var err error
	var session TftpSession

	// Do not derive new context
	session.Ctx = ctx

	// Default values
	session.BlockSize = 512
	session.Timeout = time.Second * 5
	session.TransferSize = 0 // 0 if unknown
	session.WindowSize = 512

	session.DestinationAddr = destination
	session.Destination, err = net.DialUDP("udp", nil, session.DestinationAddr)
	if err != nil {
		return TftpSession{}, err
	}
	// Add 4 to support the data message preamble
	session.SendBuf = make([]byte, session.BlockSize+DataPreambleLength)
	session.ReceiveBuf = make([]byte, session.BlockSize+DataPreambleLength)

	session.Options = make(map[string]string)

	return session, nil
}

func (session *TftpSession) Close() error {
	return session.Destination.Close()
}

func (session *TftpSession) LastSentMessageType() uint16 {
	opcodeLen := 2
	if session.SendBuf == nil || len(session.SendBuf) < opcodeLen {
		return OpcodeInvalid
	}

	return uint16(session.SendBuf[1])
}

// open file and increment consumers attached to that file
func (session *TftpSession) Reserve() (int64, error) {
	var model fileModel = newFileModel()

	// Acquire write lock on global map
	tx, err := DB.BeginTx(session.Ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		return 0, err
	}

	stmt := tx.Stmt(ReserveStatementRead)
	row := stmt.QueryRow(session.Filename)
	err = model.scanRow(row)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	unixMicro := model.timeStarted

	stmt = tx.Stmt(ReserveStatementWrite)
	_, err = stmt.Exec(session.Filename)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	unixMicro = model.timeStarted

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	file, err := Cfg.Directory.Open(model.Path())
	if err != nil {
		return 0, err
	}
	session.File = file

	return unixMicro, nil
}

// close file and decrement consumers attached to that file.
// called from within sessionRoutine because both when loading options
// or responding to a read request we may open a file for the first time.
func (session *TftpSession) Release(unixMicro int64) error {
	var model fileModel = newFileModel()

	// file may be nil, means nothing was ever opened
	if session.File == nil {
		return nil
	}

	session.File.Close()

	tx, err := DB.BeginTx(session.Ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		return err
	}

	stmt := tx.Stmt(ReleaseStatementRead)
	row := stmt.QueryRow(session.Filename)
	err = model.scanRow(row)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_ = model.timeStarted

	stmt = tx.Stmt(ReleaseStatementWrite)
	_, err = stmt.Exec(session.Filename)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// inform databse we want to begin writing a version of filename and get a time attached to it
func (session *TftpSession) Prepare() (int64, error) {
	var model fileModel = newFileModel()

	tx, err := DB.BeginTx(session.Ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		return 0, err
	}
	unixMicro := time.Now().UnixMicro()
	stmt := tx.Stmt(PrepareStatement)
	_, err = stmt.Exec(session.Filename, unixMicro)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	model = newFileModelWith(session.Filename, unixMicro, 0, 0)
	file, err := Cfg.Directory.Create(model.Path())
	if err != nil {
		return 0, err
	}

	session.File = file
	return unixMicro, err
}

// inform database client succesfully uploaded entire file, mark it as available
func (session *TftpSession) OverwriteSuccess(unixMicro int64) error {
	err := session.File.Close()
	if err != nil {
		return err
	}

	tx, err := DB.BeginTx(session.Ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		return err
	}
	stmt := tx.Stmt(OverwriteSuccessStatement)
	uploadCompleted := time.Now().UnixMicro()
	_, err = stmt.Exec(uploadCompleted, session.Filename, unixMicro)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// inform database client succesfully uploaded entire file, mark it as available
func (session *TftpSession) OverwriteFailure(unixMicro int64) error {
	// When err is nil then some error has prevented the file from being written as it should have been
	// When err is os.ErrClosed then overwriteSuccess has already been called and all is good
	// When err is any other error then something weird has happened
	fileError := session.File.Close()
	if errors.Is(fileError, os.ErrClosed) {
		return nil
	} else if fileError != nil {
		return fileError
	}

	tx, err := DB.BeginTx(session.Ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		return err
	}
	stmt := tx.Stmt(OverwriteFailureStatement)
	_, err = stmt.Exec(session.Filename, unixMicro)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (session *TftpSession) ReadFile() error {
	if len(session.SendBuf) <= DataPreambleLength {
		return errors.New("Buffer size way too small to read")
	}

	// somewhat hacky way to avoid double copying
	// we let MessageAsBytes handle the first 4 bytes and handle the rest by reading
	err := MessageAsBytes(NewDataMessage(session.BlockNumber, nil), &session.SendBuf)
	if err != nil {
		return err
	}

	session.SendBuf = session.SendBuf[:session.BlockSize+DataPreambleLength]
	n, err := session.File.Read(session.SendBuf[DataPreambleLength:])
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	// Need to adjust session.SendBuf slice length
	// Writing bytes to session.SendBuf[DataPreambleLen:] does not update session.SendBuf itself
	newSliceEnd := DataPreambleLength + n
	session.SendBuf = session.SendBuf[0:newSliceEnd]
	return err
}

// assumption made is that session.SendBuf already contains a data message as bytes
func (session *TftpSession) WriteFile() error {
	body := session.MostRecentMessage.(DataMessage).Body

	// Zero length data section means previous message was the last message containing file data
	if len(body) == 0 {
		return io.EOF
	}

	// somewhat hacky way to avoid double copying
	// We know that bytes 5 and onward must be actual data being sent
	n, err := session.File.Write(body)
	if n != len(body) {
		return errors.New("Truncated")
	}
	if err != nil {
		return err
	}

	// Short message means end of file
	if n < int(session.BlockSize) {
		err = io.EOF
	}
	return err
}

// send length bytes of session.SendBuf to client
func (session *TftpSession) Send() error {
	if session == nil || session.SendBuf == nil {
		return nil
	}

	n, err := session.Destination.Write(session.SendBuf)
	if err != nil {
		return err
	}
	if n != len(session.SendBuf) {
		return errors.New("Truncated network send")
	}

	return nil
}

func (session *TftpSession) Receive() error {
	// Allow ten timeouts, if we timeout then resend
	for _ = range 10 {
		select {
		case <-session.Ctx.Done():
			return context.Cause(session.Ctx)
		default:
			session.ReceiveBuf = session.ReceiveBuf[:session.BlockSize+DataPreambleLength]
			session.Destination.SetReadDeadline(time.Now().Add(session.Timeout))
			messageLength, addr, err := session.Destination.ReadFromUDP(session.ReceiveBuf)
			session.ReceiveBuf = session.ReceiveBuf[:messageLength]
			if err != nil {
				return err
			}
			ip1, ip2 := addr.IP, session.DestinationAddr.IP
			port1, port2 := addr.Port, session.DestinationAddr.Port
			zone1, zone2 := addr.Zone, session.DestinationAddr.Zone
			if !ip1.Equal(ip2) || port1 != port2 || zone1 != zone2 {
				return errors.New("Client changed ip/port... possible man in the middle attack?")
			}
			session.MostRecentMessage, err = BytesAsMessage(session.ReceiveBuf[:messageLength])
			if err != nil {
				return err
			}
			return nil
		}
	}

	return errors.New("Client connection (likely) dead")
}

// create and send read message to server
func (session *TftpSession) ReadMessage(filename string, options map[string]string) error {
	// netascii is a nop
	mode := "octal"

	err := MessageAsBytes(NewReadMessage(filename, mode, options), &(session.SendBuf))
	if err != nil {
		return err
	}

	session.Filename = filename
	session.Options = options
	return session.Send()
}

// create and send write message to server
func (session *TftpSession) WriteMessage(filename string, options map[string]string) error {
	// netascii is a nop
	mode := "octal"

	err := MessageAsBytes(NewWriteMessage(filename, mode, options), &(session.SendBuf))
	if err != nil {
		return err
	}

	session.Filename = filename
	session.Options = options
	return session.Send()
}

// create and send data message to client
func (session *TftpSession) DataMessage() error {
	// session.SendBuf used to store loaded data so special care taken to avoid unneeded secondary write
	// see session.FileRead to learn how data is put into proper order

	return session.Send()
}

// create and send acknowledge message to client
func (session *TftpSession) AcknowledgeMessage() error {
	acknowledgeOpcodeLen := 2
	acknowledgeBlockNumLen := 2

	err := MessageAsBytes(NewAcknowledgeMessage(session.BlockNumber), &session.SendBuf)
	if err != nil {
		return err
	}
	if len(session.SendBuf) != acknowledgeOpcodeLen+acknowledgeBlockNumLen {
		return errors.New("Truncated acknowledge message")
	}

	return session.Send()
}

// create and send error message to client
func (session *TftpSession) ErrorMessage(code uint8, message string) error {
	errorOpcodeLen := 2
	errorCodeLen := 2
	nullTerminatorLen := 1

	err := MessageAsBytes(NewErrorMessage(uint16(code), message), &session.SendBuf)
	if err != nil {
		return err
	}
	if len(session.SendBuf) != errorOpcodeLen+errorCodeLen+len(message)+nullTerminatorLen {
		return errors.New("Truncated error message")
	}

	return session.Send()
}

// create and send option acknowledge message to client
func (session *TftpSession) OptionAcknowledgeMessage() error {
	optionAcknowledgeOpcodeLen := 2

	err := MessageAsBytes(NewOptionAcknowledgeMessage(session.Options), &session.SendBuf)
	if err != nil {
		return err
	}
	expectedTotal := 0
	for key, value := range session.Options {
		expectedTotal += len(key) + 1 + len(value) + 1
	}
	if len(session.SendBuf) != optionAcknowledgeOpcodeLen+expectedTotal {
		return errors.New("Truncated option acknowledge message")
	}

	return session.Send()
}

// establish connection with client
// only valid messages client can use to initiate connection are readMessage and writeMessage
// check client options to see if they are valid
func (session *TftpSession) Accept(bytes []byte) (uint16, error) {
	var err error

	session.MostRecentMessage, err = BytesAsMessage(bytes)
	if err != nil {
		return OpcodeInvalid, errors.New("Client sent unknown message type when opening connecting")
	}

	switch session.MostRecentMessage.(type) {
	case ReadMessage:
		session.Operation = ReadAsServer
		session.Filename = session.MostRecentMessage.(ReadMessage).Filename
		session.Mode = session.MostRecentMessage.(ReadMessage).Mode

	case WriteMessage:
		session.Operation = WriteAsServer
		session.Filename = session.MostRecentMessage.(WriteMessage).Filename
		session.Mode = session.MostRecentMessage.(WriteMessage).Mode

	default:
		session.ErrorMessage(ErrorCodeIllegalOperation, "Client requested invalid operation when opening connection")
		return OpcodeInvalid, errors.New("Client requested invalid operation when opening connection")
	}

	return session.Operation, nil
}

// add heldMessage argument to receiveDataLoop and sendDataLoop
// add field to session used to check whether session is acting as a server or client

/*
func (session *TftpSession) ReadAsClient(filename string, options map[string]string) error {
    var err error

    if err = session.ReadMessage(); err != nil {
        session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
        Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Session routine failed to accept: %v", err)
        return err
    }

    // set tsize is 0 in order to get the expected  size from the server
    //  call receive right now making the assumption to handle options {
    //    :server_does => compare those options with current set options and send ack if they are good
    //    :server_does_not => proceed to receiveDataLoop
    //  }
    //
    //  open file (no need for worrying about filemodel
    //  defer file.Close()
    //
    //  call receiveDataLoop using new `heldMessage` argument used to indicate we already hold some message
}
*/

func (session *TftpSession) ReadAsServer() error {
	var err error

	session.UnixMicro, err = session.Reserve()
	if err != nil {
		return errors.New("File does not exist!")
	}
	defer session.Release(session.UnixMicro)

	err = session.UpdateOptions(session.MostRecentMessage.(ReadMessage).Options)
	if err != nil {
		session.ErrorMessage(ErrorCodeUndefined, "One or more options contain invalid values")
		return errors.New("One or more options contain invalid values")
	}

	if len(session.Options) > 0 {
		if err = session.OptionAcknowledgeMessage(); err != nil {
			return errors.New("Unable to send option acknowledgement to write request")
		}
	}

	// If the client asked for options we may have already sent an options acknowledge message
	// If we have just sent an options acknowledgement message we need to operate on the client's acknowledgement message
	switch session.LastSentMessageType() {
	case OpcodeOptionAcknowledgeByte:
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), "Awaiting acknowledgement from client of option acknowledge message")
		}
		session.LastValidMessage = session.MostRecentMessage
		if err = session.Receive(); err != nil {
			return err
		}

		switch session.MostRecentMessage.(type) {
		case AcknowledgeMessage:
			// pass
		default:
			return errors.New("Did not acknowledge the server's attempt to open a connection with supplied options")
		}

		if session.MostRecentMessage.(AcknowledgeMessage).BlockNumber != 0 {
			return errors.New("Did not acknowledge the server's attempt to open a connection with supplied options")
		}

		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), "Received acknowledgement from client of option acknowledge message")
		}
	default:
		// pass
	}

	// we expect that client will acknowledge first data block with 1
	session.BlockNumber = 1

	// adjust buffer size because of blockSize
	if len(session.SendBuf) != int(DataPreambleLength+session.BlockSize) {
		session.SendBuf = make([]byte, session.BlockSize+DataPreambleLength)
	}
	if len(session.ReceiveBuf) != int(DataPreambleLength+session.BlockSize) {
		session.ReceiveBuf = make([]byte, DataPreambleLength+session.BlockSize)
	}

	// get access to file with associated time
	if Cfg.Debug {
		Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Reserving %v", session.Filename))
	}
	if Cfg.Debug {
		Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Reserved %v with file time %v", session.Filename, session.UnixMicro))
	}

	if err = session.SendDataLoop(); err != nil {
		return err
	}

	return nil
}

func (session *TftpSession) SendDataLoop() error {
	var readEverything bool = false
	var err error

	// for loop designed to deal with multiple possible client messages
	// if the client sends an acknowledgeMessage then we should send the next data message
	// if the client sends an errorMessage then we log it and return error
	// if the client sends anything else return error
	for !readEverything {
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Preparing data message with block number #%v", session.BlockNumber))
		}
		err = session.ReadFile()
		if errors.Is(err, io.EOF) || len(session.SendBuf) < int(DataPreambleLength+session.BlockSize) {
			readEverything = true
		} else if err != nil {
			return errors.New("File read error")
		}
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Prepared data message with block number #%v", session.BlockNumber))
		}

		if session.DataMessage() != nil {
			return errors.New("Unable to send data to client")
		}

		// setup for receive loop
		session.LastValidMessage = session.MostRecentMessage
		var i int = 1
		var awaitingRequest bool = true

		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Awaiting client acknowledgement of block #%v", session.BlockNumber))
		}
		// read until acknowledgement with correct blockNumber, handling gracefully retransmissions
		for awaitingRequest {
			// timeout after five bad messages
			if i > 5 {
				// don't tell client so as to avoid further network issues
				return errors.New("Underlying network may be bad, many retransmiteed messages")
			}

			// Get next potentially valid message
			if err = session.Receive(); err != nil {
				return err
			}

			// Read messages or acknowledgements of lesser block numbers are treated as retransmissions
			switch session.MostRecentMessage.(type) {
			case ReadMessage:
				// pass
			case AcknowledgeMessage:
				if session.MostRecentMessage.(AcknowledgeMessage).BlockNumber == session.BlockNumber {
					awaitingRequest = false
				} else if session.MostRecentMessage.(AcknowledgeMessage).BlockNumber > session.BlockNumber {
					return errors.New("Out of sync blockNumber")
				}
			case ErrorMessage:
				return errors.New(session.MostRecentMessage.(ErrorMessage).Explanation)
			default:
				return errors.New("Client requested invalid operation during established connection")
			}

			i += 1
		}
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Client acknowledged block #%v", session.BlockNumber))
		}

		session.BlockNumber += 1
	}

	return nil
}

func (session *TftpSession) WriteAsServer() error {
	var unixMicro int64
	var err error

	err = session.UpdateOptions(session.MostRecentMessage.(WriteMessage).Options)
	if err != nil {
		session.ErrorMessage(ErrorCodeUndefined, "One or more options contain invalid values")
		return errors.New("One or more options contain invalid values")
	}

	// acknowledgement message with block number 0 used to indicate accepting write when options are empty
	session.BlockNumber = 0

	if len(session.Options) > 0 {
		if err = session.OptionAcknowledgeMessage(); err != nil {
			return errors.New("Unable to send option acknowledgement to write request")
		}
	} else if err = session.AcknowledgeMessage(); err != nil {
		return errors.New("Unable to send acknowledgement to write request")
	}

	// we expect that client will acknowledge first data block with 1
	session.BlockNumber = 1

	// adjust buffer size because of blockSize
	if len(session.SendBuf) != int(DataPreambleLength+session.BlockSize) {
		session.SendBuf = make([]byte, session.BlockSize+DataPreambleLength)
	}
	if len(session.ReceiveBuf) != int(DataPreambleLength+session.BlockSize) {
		session.ReceiveBuf = make([]byte, DataPreambleLength+session.BlockSize)
	}

	// get access to a file and associated time
	if Cfg.Debug {
		Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Preparing %v", session.Filename))
	}
	unixMicro, err = session.Prepare()
	if err != nil {
		return err
	}
	defer session.OverwriteFailure(unixMicro)
	if Cfg.Debug {
		Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Prepared %v with file time %v", session.Filename, unixMicro))
	}

	err = session.ReceiveDataLoop()
	if err != nil {
		return err
	}

	// It is okay to try to close an already closed file, the second close just fails
	err = session.OverwriteSuccess(unixMicro)
	if err != nil {
		return err
	}

	return nil
}

func (session *TftpSession) ReceiveDataLoop() error {
	var wroteEverything bool = false
	var err error

	// if the client sends an dataMessage then we should acknowledge it
	// if the client sends an errorMessage then we log it and return error
	// if the client sends anything else return error
	for !wroteEverything {
		// setup for receive loop
		session.LastValidMessage = session.MostRecentMessage
		var i int = 1
		var awaitingRequest = true

		// read until acknowledgement with correct blockNumber, handle gracefully retransmission
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Awaiting client data block #%v", session.BlockNumber))
		}
		for awaitingRequest {
			if i > 5 {
				return errors.New("Underlying network may be bad, many retransmiteed messages")
			}

			if err = session.Receive(); err != nil {
				return err
			}

			switch session.MostRecentMessage.(type) {
			case WriteMessage:
				// pass
			case DataMessage:
				if session.MostRecentMessage.(DataMessage).BlockNumber == session.BlockNumber {
					awaitingRequest = false
				} else if session.MostRecentMessage.(DataMessage).BlockNumber > session.BlockNumber {
					return errors.New("Out of sync blockNumber")
				}
			case ErrorMessage:
				return errors.New(session.MostRecentMessage.(ErrorMessage).Explanation)
			default:
				return errors.New("Client requested invalid operation during established connection")
			}
		}
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Client sent data block #%v", session.BlockNumber))
		}

		// write to file
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Writing data message with block number #%v", session.BlockNumber))
		}
		err = session.WriteFile()
		if errors.Is(err, io.EOF) {
			wroteEverything = true
		} else if err != nil {
			return err
		}
		if Cfg.Debug {
			Log <- NewDebugEvent(session.DestinationAddr.String(), fmt.Sprintf("Wrote data message with block number #%v", session.BlockNumber))
		}

		// acknowledge
		if err = session.AcknowledgeMessage(); err != nil {
			return err
		}

		session.BlockNumber += 1
	}

	return nil
}

func (session *TftpSession) UpdateOptions(options map[string]string) error {
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
				session.BlockSize = uint16(valueInt)
				options["blksize"] = valueAscii
			}
		case "timeout":
			if valueInt < 1 || valueInt > 255 {
				// refer to original specification to learn about what should be done in this case
			} else {
				session.Timeout = time.Second * time.Duration(valueInt)
				options["timeout"] = valueAscii
			}
		case "tsize":
			// tsize of 0 as in read request is special
			// server responds in optionAcknowledge message with size of file
			if valueInt == 0 {
				if session.Operation == ReadAsServer {
					model := newFileModelWith(session.Filename, session.UnixMicro, 0, 0)
					info, err := Cfg.Directory.Lstat(model.Path())
					if err != nil {
						return errors.New(fmt.Sprintf("Unable to get the size of %v", session.Filename))
					}
					valueInt = info.Size()
				} else {
					return errors.New(fmt.Sprintf("Invalid blksize value %v requested by client", valueInt))
				}
			}

			session.TransferSize = uint64(valueInt)
			options["tsize"] = strconv.FormatInt(valueInt, 10)
		case "multicast":
			// not implement
			continue
		case "windowsize":
			continue
			//if valueInt < 1 || valueInt > 65535 {
			// refer to original specification to learn about what should be done in this case
			//}
			//session.WindowSize = uint16(valueInt)
			//options["windowsize"] = valueAscii
		default:
			continue
		}
	}

	session.Options = options

	return nil
}
