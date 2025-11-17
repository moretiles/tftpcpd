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
	Cfg.Directory, err = os.OpenRoot(".")
	if err != nil {
		fmt.Println("Failed to setup environment for tests")
	}
	Cfg.Sqlite3DBPath = "test.tftpcpd.db"

	if LoggerInit() != nil {
		os.Exit(4)
	}
	if DatabaseInit() != nil {
		os.Exit(2)
	}
	if ServerInit() != nil {
		os.Exit(3)
	}
}
