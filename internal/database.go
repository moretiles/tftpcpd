package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"strconv"
	"time"
)

type fileModel struct {
	filename      string
	timeStarted   int64
	timeCompleted int64
	consumers     int64
}

func newFileModel() fileModel {
	return fileModel{}
}

func newFileModelWith(filename string, timeStarted, timeCompleted, consumers int64) fileModel {
	return fileModel{filename, timeStarted, timeCompleted, consumers}
}

func (model fileModel) Path() string {
	return model.filename + "." + strconv.FormatInt(model.timeStarted, 10)
}

func (model *fileModel) deleteFiles(ctx context.Context, rows *sql.Rows) error {
	for rows.Next() {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		default:
			err := model.scanRows(rows)
			if err != nil {
				return err
			}

			err = Cfg.Directory.Remove(model.Path())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func (model *fileModel) scanRows(rows *sql.Rows) error {
	return rows.Scan(&(model.filename), &(model.timeStarted), &(model.timeCompleted), &(model.consumers))
}

func (model *fileModel) scanRow(row *sql.Row) error {
	return row.Scan(&(model.filename), &(model.timeStarted), &(model.timeCompleted), &(model.consumers))
}

func deleteOutOfDateGlobally() error {
	var model fileModel = newFileModel()
	var err error

	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(3*time.Minute))

	// start new transaction
	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// select all files for which a more recent version already exists and are not being uploaded or consumed, returning the deleted rows
	rows, err := tx.QueryContext(ctx, `SELECT * FROM files WHERE
            uploadCompleted != 0 AND
            consumers = 0 AND
            (rowid, filename, uploadCompleted) NOT IN ( SELECT rowid, filename, MAX(uploadCompleted) FROM files GROUP BY filename );`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	defer rows.Close()
	err = model.deleteFiles(ctx, rows)
	if err != nil {
		return err
	}

	// delete all files for which a more recent version already exists and are not being uploaded or consumed, returning the deleted rows
	_, err = tx.ExecContext(ctx, `DELETE FROM files WHERE
            uploadCompleted != 0 AND
            consumers = 0 AND
            (rowid, filename, uploadCompleted) NOT IN ( SELECT rowid, filename, MAX(uploadCompleted) FROM files GROUP BY filename )`)
	if err != nil {
		_ = tx.Rollback()
		// die
	}

	// commit new transaction
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func DatabaseInit() error {
	var err error
	var errPtr *error = &err
	var parentCtx context.Context = context.Background()

	// only run first time databaseRoutine itself starts
	DB, err = sql.Open("sqlite3", Cfg.Sqlite3DBPath)
	if err != nil {
		Log <- NewErrorEvent("DATABASE", fmt.Sprintf("Unable to open database file at: %v", Cfg.Sqlite3DBPath))
		return err
	}
	defer func() {
		if *errPtr != nil {
			Log <- NewErrorEvent("DATABASE", fmt.Sprintf("Encountered error opening database: %v", err))
			DB.Close()
		}
	}()

	// Prepare DB if not already open
	{
		var tx *sql.Tx

		// make sure database is working
		ctx, _ := context.WithDeadline(parentCtx, time.Now().Add(time.Second*3))
		err = DB.PingContext(ctx)
		if err != nil {
			return err
		}

		ctx, _ = context.WithDeadline(parentCtx, time.Now().Add(time.Second*15))
		tx, err = DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		// create files table
		_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS files(filename STRING, uploadStarted INT UNIQUE, uploadCompleted INT, consumers INT);`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		// create files_name index
		_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS files_filename ON files(filename);`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		err = tx.Commit()
		if err != nil {
			return err
		}
	}

	// clear failed uploads and zero out consumers
	{
		var tx *sql.Tx
		var rows *sql.Rows

		ctx, _ := context.WithDeadline(parentCtx, time.Now().Add(3*time.Minute))
		tx, err = DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		// get all failed uploads
		rows, err = tx.QueryContext(ctx, `SELECT * FROM files WHERE uploadCompleted = 0;`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		defer rows.Close()
		var model = newFileModel()
		err = model.deleteFiles(ctx, rows)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		_, err = tx.ExecContext(ctx, `DELETE FROM files WHERE uploadCompleted = 0;`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		// zero out consumers
		_, err = tx.ExecContext(ctx, `UPDATE files SET consumers = 0;`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		err = tx.Commit()
		if err != nil {
			return err
		}
	}

	err = deleteOutOfDateGlobally()
	if err != nil {
		return err
	}

	return nil
}

// listen for requests to terminate from parent and clear out-of-date files not being requested
func DatabaseRoutine(childToParent chan<- Signal, parentToChild <-chan Signal) {
	//var err error

	Log <- NewNormalEvent("DATABASE", fmt.Sprintf("Database ready for access: %v", Cfg.Sqlite3DBPath))

	for true {
		select {
		case sig := <-parentToChild:
			childToParent <- NewSignal(sig.Kind, SignalAccept)

			// try to clear before exiting
			_ = deleteOutOfDateGlobally()
			return
			// If reads and writes by sessions themselves delete out of date files then we do not need this routine to lock and scan the entire files tables

			/*
				case <-time.After(15 * time.Minute):
					err = deleteOutOfDateGlobally()
					if err != nil {
						childToParent <- NewSignal(SignalTerminate, SignalRequest)
						<-parentToChild
						return
					}
			*/
		}
	}
}
