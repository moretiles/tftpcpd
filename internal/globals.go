package internal

import (
	"database/sql"
	"os"
)

type Config struct {
	// behavior
	MemoryLimit int
	Debug       bool

	// files
	Directory     *os.Root
	Sqlite3DBPath string
	NormalLogFile string
	DebugLogFile  string
	ErrorLogFile  string

	// args
	Address string

	// used to check whether we are testing
	Testing *bool
}

// Globals in this conext are all variables used across multiple files that mutate
var ReserveStatementRead *sql.Stmt
var ReserveStatementWrite *sql.Stmt
var ReleaseStatementRead *sql.Stmt
var ReleaseStatementWrite *sql.Stmt
var PrepareStatement *sql.Stmt
var OverwriteSuccessStatement *sql.Stmt
var OverwriteFailureStatement *sql.Stmt

// Used by everything
var Cfg Config = Config{}

// Read from by logRoutine, written to by everyone
var Log chan logEvent = make(chan logEvent, 150)

// Managing reads and writes to database
var DB *sql.DB = nil
