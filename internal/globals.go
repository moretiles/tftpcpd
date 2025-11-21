package internal

import (
	"database/sql"
	"os"
)

type Config struct {
	// behavior
	MemoryLimit int
	Debug       bool

	// server options
	Directory     *os.Root
	Sqlite3DBPath string
	NormalLogFile string
	DebugLogFile  string
	ErrorLogFile  string

	// client options
	Write string

	// args
	Address  string
	Filename string

	// used to check whether we are testing
	Testing *bool
}

// Globals in this conext are all variables used across multiple files that mutate
var ReserveStatementSelect *sql.Stmt
var ReserveStatementUpdate *sql.Stmt
var ReleaseStatementSelect *sql.Stmt
var ReleaseStatementUpdate *sql.Stmt
var ReleaseStatementDelete *sql.Stmt
var PrepareStatement *sql.Stmt
var OverwriteSuccessSelect *sql.Stmt
var OverwriteSuccessUpdate *sql.Stmt
var OverwriteSuccessDelete *sql.Stmt
var OverwriteFailureStatement *sql.Stmt

// Used by everything
var Cfg Config = Config{}

// Read from by logRoutine, written to by everyone
var Log chan logEvent = make(chan logEvent, 150)

// Managing reads and writes to database
var DB *sql.DB = nil
