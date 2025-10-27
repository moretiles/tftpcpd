package main

import (
	"container/list"
	"sync"
)

// All variables used across multiple files that mutate

var cfg config = config{}

var log chan logEvent = make(chan logEvent, 150)
var terminate chan bool

var activeReads map[string]list.List
var activeReadsMutex sync.RWMutex
