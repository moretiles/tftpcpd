package main

import (
	"container/list"
	"sync"
)

// Globals in this conext are all variables used across multiple files that mutate

var cfg config = config{}

var log chan logEvent = make(chan logEvent, 150)

var inUseFiles map[string]list.List
var inUseFilesMutex sync.RWMutex
