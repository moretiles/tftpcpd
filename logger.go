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

func loggerRoutine(childToParent chan<- Signal, parentToChild <-chan Signal) {
	normalMessageLog := os.Stdout
	debugMessageLog := os.Stderr
	errorMessageLog := os.Stderr
	var normalMessageLogError, debugMessageLogError, errorMessageLogError error
	if cfg.normalLogFile != "" {
		normalMessageLog, normalMessageLogError = os.OpenFile(cfg.normalLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		defer normalMessageLog.Close()
	}
	if cfg.debugLogFile != "" {
		debugMessageLog, debugMessageLogError = os.OpenFile(cfg.debugLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		defer debugMessageLog.Close()
	}
	if cfg.errorLogFile != "" {
		errorMessageLog, errorMessageLogError = os.OpenFile(cfg.errorLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		defer errorMessageLog.Close()
	}
	if normalMessageLogError != nil || debugMessageLogError != nil || errorMessageLogError != nil {
		fmt.Fprintln(os.Stderr, "Unable to open one or more files logging was requested to")
		childToParent <- NewSignal(SignalTerminate, SignalRequest)
		<-parentToChild

		return
	}

	for true {
		select {
		case event, isOpen := <-log:
			if !isOpen {
				// signal to main that we want to terminate this routine and all others
				childToParent <- NewSignal(SignalTerminate, SignalRequest)
				<-parentToChild

				return
			} else {
				writeEventToLog(event, normalMessageLog, debugMessageLog, errorMessageLog)
			}
		default:
			select {
			case sig := <-parentToChild:
				childToParent <- NewSignal(sig.Kind, SignalAccept)
				return
			default:
				// pass
			}
		}
	}
}

func writeEventToLog(event logEvent, normalMessageLog, debugMessageLog, errorMessageLog *os.File) {
	switch event.kind {
	case normalMsg:
		fmt.Fprintln(normalMessageLog, event)
	case debugMsg:
		fmt.Fprintln(debugMessageLog, event)
	case errorMsg:
		fmt.Fprintln(errorMessageLog, event)
	default:
		log <- newErrorEvent("LOGGER", fmt.Sprintf("Malformed log partial: %v", event.message))
	}
}
