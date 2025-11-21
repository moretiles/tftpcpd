//go:build test
// +build test

package internal

import (
	"fmt"
	"os"
)

func init() {
	var err error

	fmt.Println("Called init function in test.go to prepare environment for tests")

	// OS reclaims this when the program closes, still annoying there is no init equivelent for once everything has finished executing
	Cfg.Directory, err = os.OpenRoot("tests")
	if err != nil {
		fmt.Println("Failed to setup environment for tests")
	}

	if LoggerInit() != nil {
		os.Exit(2)
	}
	if DatabaseInit() != nil {
		os.Exit(3)
	}
	if ServerInit() != nil {
		os.Exit(4)
	}
}
