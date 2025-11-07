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
	defer close(log)
	defer cfg.directory.Close()

	var (
		loggerDemandTermination  chan bool = make(chan bool, 2)
		loggerConfirmTermination chan bool = make(chan bool, 2)
		serverDemandTermination  chan bool = make(chan bool, 2)
		serverConfirmTermination chan bool = make(chan bool, 2)
		//databaseDemandTermination chan bool = make(chan bool, 2)
		//databaseConfirmTermination chan bool = make(chan bool, 2)
		//fileWriteStarted  chan string   = make(chan string)
		//fileWriteFinished chan string   = make(chan string)
		wg sync.WaitGroup
	)
	defer close(loggerDemandTermination)
	defer close(loggerConfirmTermination)
	defer close(serverDemandTermination)
	defer close(serverConfirmTermination)
	//defer close(databaseDemandTermination)
	//defer close(databaseConfirmTermination)
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
			serverRoutine(serverDemandTermination, serverConfirmTermination)
		})

		// logger routine collects logs
		wg.Go(func() {
			loggerRoutine(loggerDemandTermination, loggerConfirmTermination)
		})

		// database routine cleans up database and files periodically
		//wg.Go(databaseRoutine(fileWriteStarted, fileWriteFinished))
	}

	// handle child goroutines terminating
	{
		select {
		case <-loggerDemandTermination:
			serverDemandTermination <- true
			<-serverConfirmTermination

			loggerConfirmTermination <- true

		case <-serverDemandTermination:
			serverConfirmTermination <- true

			loggerDemandTermination <- true
			<-loggerConfirmTermination

			// Make sure we respond to demands to termiante correctly
			//case <- time.After(1 * time.Second):
			//fmt.Println("Time to exit logger")
			//serverDemandTermination <- true
			//<- serverConfirmTermination
			//fmt.Println("Time to exit server")
			//loggerDemandTermination <- true
			//<- loggerConfirmTermination
			//fmt.Println("All exited")
		}
	}

	// wait for all goroutines to finished
	wg.Wait()
}
