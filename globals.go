package main

import (
	"database/sql"
)

// Globals in this conext are all variables used across multiple files that mutate
var reserveStatementRead *sql.Stmt
var reserveStatementWrite *sql.Stmt
var releaseStatementRead *sql.Stmt
var releaseStatementWrite *sql.Stmt
var prepareStatement *sql.Stmt
var overwriteSuccessStatement *sql.Stmt
var overwriteFailureStatement *sql.Stmt

// Used by everything
var cfg config = config{}

// Read from by logRoutine, written to by everyone
var log chan logEvent = make(chan logEvent, 150)

// Managing reads and writes to database
var db *sql.DB = nil
