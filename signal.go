package main

type Signal struct {
	Kind    uint8
	Message uint8
}

// Possible values for kind
const (
	SignalTerminate = iota
	SignalRestart
)

// Possible values for message
const (
	// Sender iniates conversation
	SignalRequest = iota // I would like to do this
	SignalDemand         // I am now doing this, good luck!

	// Response to sender
	SignalAccept // go ahead (:
	SignalDeny   // stop!
)

func NewSignal(kind, message uint8) Signal {
	return Signal{kind, message}
}

func (sig *Signal) IsRequest() bool {
	return sig.Kind == SignalRequest || sig.Kind == SignalDemand
}

func (sig *Signal) IsResponse() bool {
	return sig.Kind == SignalAccept || sig.Kind == SignalDeny
}
