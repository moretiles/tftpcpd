package main

import (
	"flag"
	"fmt"
	"path/filepath"
	//"net"
	"os"
	//"reflect"
	"sync"
	//"time"
	//"errors"
	//"strings"
	_ "database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type config struct {
	// flags
	directory   *os.Root
	memoryLimit int
	debug       bool

	// logs
	normalLogFile string
	debugLogFile  string
	errorLogFile  string

	// args
	address string

	// used to check whether we are testing
	testing *bool
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
	cfg.testing = flag.Bool("testing", false, "Used to control special behavior required for running tests.")
}

func main() {
	var (
		loggerParentToChild chan Signal = make(chan Signal, 2)
		loggerChildToParent chan Signal = make(chan Signal, 2)
		serverParentToChild chan Signal = make(chan Signal, 2)
		serverChildToParent chan Signal = make(chan Signal, 2)
		//databaseParentToChild chan Signal = make(chan Signal, 2)
		//databaseChildToParent chan Signal = make(chan Signal, 2)
		//fileWriteStarted  chan string   = make(chan string)
		//fileWriteFinished chan string   = make(chan string)
		wg sync.WaitGroup
        exitCode int
	)
	defer close(loggerParentToChild)
	defer close(loggerChildToParent)
	defer close(serverParentToChild)
	defer close(serverChildToParent)
	//defer close(databaseParentToChild)
	//defer close(databaseChildToParent)
	//defer close(fileWriteStarted)
	//defer close(fileWriteFinished)

	// setup configuration using commandline arguments
	{
		// Set flag.Usage to change default help message
		oldFlagUsageFunction := flag.Usage
		flag.Usage = func() { helpMessage(oldFlagUsageFunction) }

		var help *bool = flag.Bool("help", false, "print usage information")

		var directory *string = flag.String("directory", ".", "root directory of server")

		var debug *bool = flag.Bool("debug", false, "enable debug mode")
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
			fmt.Fprintln(os.Stderr, newErrorEvent("CONFIG", fmt.Sprintf("Unable to open root directory as absolute path: %v ", *directory)))
			os.Exit(1)
		}
		cfg.directory, err = os.OpenRoot(absoluteDirectory)
		if err != nil {
			fmt.Fprintln(os.Stderr, newErrorEvent("CONFIG", fmt.Sprintf("Unable to open root directory: %v", absoluteDirectory)))
			os.Exit(1)
		}
		log <- newNormalEvent("CONFIG", fmt.Sprintf("Ready to serve as root directory: %v", absoluteDirectory))

		cfg.debug = *debug
		cfg.normalLogFile = *normalLogFile
		cfg.debugLogFile = *debugLogFile
		cfg.errorLogFile = *errorLogFile

		args := flag.Args()

		if len(args) == 0 {
			cfg.address = "127.0.0.1:8173"
		} else if len(args) == 1 {
			cfg.address = args[0]
		} else {
			flag.Usage()
			os.Exit(1)
		}
	}

	// start goroutines
	{
		// echo routine simple prototype build when designing system
		//wg.Go(func() {
		//echoRoutine(serverDemandTermination, serverConfirmTermination, log)
		//})

		// server routine handles actual tftp connections made by clients
		wg.Go(func() {
			serverRoutine(serverChildToParent, serverParentToChild)
		})

		// logger routine collects logs
		wg.Go(func() {
			loggerRoutine(loggerChildToParent, loggerParentToChild)
		})

		// database routine cleans up database and files periodically
		//wg.Go(databaseRoutine(databaseChildToParent, databaseParentToChild))
	}

	// handle child goroutines terminating
	{
        exitCode = 0
		var sig Signal
		select {
		case sig = <-loggerChildToParent:
			if sig.IsResponse() {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			if sig.Kind == SignalRestart {
				loggerParentToChild <- NewSignal(SignalRestart, SignalAccept)
			} else if sig.Kind == SignalTerminate {
				serverParentToChild <- NewSignal(SignalTerminate, SignalRequest)
				<-serverChildToParent
				close(log)
				loggerParentToChild <- NewSignal(SignalTerminate, SignalAccept)
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

            exitCode = 11

		case sig = <-serverChildToParent:
			if sig.IsResponse() {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			if sig.Kind == SignalTerminate {
				serverParentToChild <- NewSignal(sig.Kind, SignalAccept)
				close(log)
				loggerParentToChild <- NewSignal(SignalTerminate, SignalRequest)
				<-loggerChildToParent
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

            exitCode = 12

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
	cfg.directory.Close()

    // Exit using code we set
    os.Exit(exitCode)
}
