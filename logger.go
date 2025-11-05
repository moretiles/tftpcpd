package main

import (
	"fmt"
	"os"
	"time"
)

const (
	debugMsg = iota
	normalMsg
	errorMsg
)

type logType uint8

type logEvent struct {
	kind    logType
	from    string
	message string
}

func newDebugEvent(from, message string) logEvent {
	return logEvent{debugMsg, from, message}
}

func newNormalEvent(from, message string) logEvent {
	return logEvent{normalMsg, from, message}
}

func newErrorEvent(from, message string) logEvent {
	return logEvent{errorMsg, from, message}
}

func (kind logType) String() string {
	switch kind {
	case debugMsg:
		return "DEBUG"
	case normalMsg:
		return "NORMAL"
	case errorMsg:
		return "ERROR"
	}

	return "???"
}

func (event logEvent) String() string {
	return fmt.Sprintf("%v %v %v %v",
		time.Now().UnixMicro(),
		event.kind,
		event.from,
		event.message)
}

func loggerRoutine(demandTermination, confirmTermination chan bool) {
	logTo := os.Stdout
	/*
		if cfg.logFile != "" {
			logTo := os.OpenFile(cfg.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
			defer(close(logTo))
		}
	*/

	for true {
		select {
		case event, isOpen := <-log:
			if !isOpen {
				// signal to main that we want to terminate this routine and all others
				demandTermination <- true
				<-confirmTermination

				// make it so that we no longer can read from this channel
				log = nil
			} else {
				fmt.Fprintf(logTo, "%v\n", event)
			}
		case <-demandTermination:
			confirmTermination <- true

			return
		}
	}
}
