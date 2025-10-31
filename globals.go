package main

import (
	"container/list"
	"sync"
)

// All variables used across multiple files that mutate

var cfg config = config{}

var log chan logEvent = make(chan logEvent, 150)
var terminate chan bool

var inUseFiles map[string]list.List
var inUseFilesMutex sync.RWMutex
