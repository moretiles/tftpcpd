package main

import (
	"flag"
	"fmt"
	"path/filepath"
	//"net"
	"os"
	"os/signal"
	//"reflect"
	"sync"
	//"time"
	//"errors"
	//"strings"
	_ "database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moretiles/tftpcpd/internal"
)

// setup configuration using commandline arguments
// no error returned because we exit early if there is a problem
func processFlags() {
	// Set flag.Usage to change default help message
	oldFlagUsageFunction := flag.Usage
	flag.Usage = func() { helpMessage(oldFlagUsageFunction) }

	// behavior
	var debug *bool = flag.Bool("debug", false, "enable debug mode")
	var help *bool = flag.Bool("help", false, "print usage information")

	// files
	var directory *string = flag.String("directory", ".", "root directory of server")
	var sqlite3DBPath *string = flag.String("sqlite3-db", "tftpcpd.db", "sqlite3 database")
	var normalLogFile *string = flag.String("normal-log", "", "log file")
	var debugLogFile *string = flag.String("debug-log", "", "debug log file")
	var errorLogFile *string = flag.String("error-log", "", "error log file")

	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	absoluteDirectory, err := filepath.Abs(*directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, internal.NewErrorEvent("CONFIG", fmt.Sprintf("Unable to open root directory as absolute path: %v ", *directory)))
		os.Exit(1)
	}
	internal.Cfg.Directory, err = os.OpenRoot(absoluteDirectory)
	if err != nil {
		fmt.Fprintln(os.Stderr, internal.NewErrorEvent("CONFIG", fmt.Sprintf("Unable to open root directory: %v", absoluteDirectory)))
		os.Exit(1)
	}
	internal.Log <- internal.NewNormalEvent("CONFIG", fmt.Sprintf("Ready to serve as root directory: %v", absoluteDirectory))

	internal.Cfg.Debug = *debug
	internal.Cfg.Sqlite3DBPath = *sqlite3DBPath
	internal.Cfg.NormalLogFile = *normalLogFile
	internal.Cfg.DebugLogFile = *debugLogFile
	internal.Cfg.ErrorLogFile = *errorLogFile

	args := flag.Args()

	if len(args) == 0 {
		internal.Cfg.Address = "127.0.0.1:8173"
	} else if len(args) == 1 {
		internal.Cfg.Address = args[0]
	} else {
		flag.Usage()
		os.Exit(1)
	}
}

func helpMessage(body func()) {
	major := 0
	minor := 0
	patch := 0

	fmt.Println("TFTPCPD: Trivial File Transfer Protocol Cross-Platform Daemon")
	fmt.Printf("v%v.%v.%v\n", major, minor, patch)
	fmt.Println("Usage:")
	fmt.Println("tftpcpd [options] hostname[:port]")
	fmt.Println("")
	body()
	fmt.Println("")
	fmt.Println("If no port is specified then the daemon binds to hostname:8173")
	fmt.Println("")
}

func init() {
	internal.Cfg.Testing = flag.Bool("testing", false, "Used to control special behavior required for running tests.")
}

func main() {
	var (
		loggerParentToChild   chan internal.Signal = make(chan internal.Signal, 2)
		loggerChildToParent   chan internal.Signal = make(chan internal.Signal, 2)
		serverParentToChild   chan internal.Signal = make(chan internal.Signal, 2)
		serverChildToParent   chan internal.Signal = make(chan internal.Signal, 2)
		databaseParentToChild chan internal.Signal = make(chan internal.Signal, 2)
		databaseChildToParent chan internal.Signal = make(chan internal.Signal, 2)
		interruptHandler      chan os.Signal       = make(chan os.Signal, 2)
		wg                    sync.WaitGroup
		exitCode              int
	)

	//locals
	defer close(loggerParentToChild)
	defer close(loggerChildToParent)
	defer close(serverParentToChild)
	defer close(serverChildToParent)
	defer close(databaseParentToChild)
	defer close(databaseChildToParent)
	defer close(interruptHandler)

	processFlags()
	if internal.LoggerInit() != nil {
		os.Exit(4)
	}
	if internal.DatabaseInit() != nil {
		os.Exit(2)
	}
	if internal.ServerInit() != nil {
		os.Exit(3)
	}
	// inform user how to exit and start goroutines
	{
		// It is a near-certainty this messagw will appear before any logs
		// good enough!
		fmt.Println("Press Control-C (^C) to exit!")

		// server routine handles actual tftp connections made by clients
		wg.Go(func() {
			internal.ServerRoutine(serverChildToParent, serverParentToChild)
		})

		// logger routine collects logs
		wg.Go(func() {
			internal.LoggerRoutine(loggerChildToParent, loggerParentToChild)
		})

		// database routine cleans up database and files periodically
		wg.Go(func() {
			internal.DatabaseRoutine(databaseChildToParent, databaseParentToChild)
		})
	}

	// handle child goroutines terminating and signals
	{
		exitCode = 0
		signal.Notify(interruptHandler, os.Interrupt)
		select {
		case <-interruptHandler:
			serverParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
			<-serverChildToParent
			databaseParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
			<-databaseChildToParent
			close(internal.Log)
			loggerParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
			<-loggerChildToParent

			// do not modify exit code because this is the expected termination method
		case sig := <-loggerChildToParent:
			if sig.IsResponse() {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			if sig.Kind == internal.SignalRestart {
				loggerParentToChild <- internal.NewSignal(internal.SignalRestart, internal.SignalAccept)
			} else if sig.Kind == internal.SignalTerminate {
				serverParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-serverChildToParent
				databaseParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-databaseChildToParent
				close(internal.Log)
				loggerParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalAccept)
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			exitCode = 11

		case sig := <-serverChildToParent:
			if sig.IsResponse() {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			if sig.Kind == internal.SignalTerminate {
				serverParentToChild <- internal.NewSignal(sig.Kind, internal.SignalAccept)
				databaseParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-databaseChildToParent
				close(internal.Log)
				loggerParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-loggerChildToParent
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			exitCode = 12

		case sig := <-databaseChildToParent:
			if sig.IsResponse() {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			if sig.Kind == internal.SignalTerminate {
				serverParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-serverChildToParent
				databaseParentToChild <- internal.NewSignal(sig.Kind, internal.SignalAccept)
				close(internal.Log)
				loggerParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalRequest)
				<-loggerChildToParent
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			// Make sure we respond to demands to termiante correctly
			//case <- time.After(1 * time.Second):
			//fmt.Println("Time to exit server")
			//serverParentToChild <- NewSignal(SignalTerminate, SignalRequest)
			//<- serverChildToParent
			//fmt.Println("Time to exit logger")
			//loggerParentToChild <- NewSignal(SignalTerminate, SignalRequest)
			//<- loggerChildToParent
			//fmt.Println("All exited")
		}
	}

	// wait for all goroutines to finished
	wg.Wait()

	//Using defer with os.Root.Close() causes panic
	//Possible bug considering os.Root is still very new?!?
	//Either way, no panic this way.
	internal.Cfg.Directory.Close()

	// Exit using code we set
	os.Exit(exitCode)
}
