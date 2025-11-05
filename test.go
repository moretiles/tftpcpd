//go:build test
// +build test

package main

import (
	"fmt"
	"os"
)

func init() {
	var err error

	fmt.Println("Called init function in test.go to prepare environment for tests")

	// OS reclaims this when the program closes, still annoying there is no init equivelent for once everything has finished executing
	cfg.directory, err = os.OpenRoot(".")
	if err != nil {
		fmt.Println("Failed to setup environment for tests")
	}

	cfg.debug = false
	cfg.memoryLimit = 0
	cfg.debug = false
	cfg.logFile = ""
}
