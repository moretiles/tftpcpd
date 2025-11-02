package main

import (
	"flag"
	"fmt"
	//"net"
	"os"
	//"reflect"
	"sync"
	//"time"
	//"errors"
	//"strings"
)

type config struct {
	// flags
	memoryLimit int
	debug       bool
	logFile     string

	// args
	address string
}

func helpMessage() {
	major := 0
	minor := 0
	patch := 0

	fmt.Println("TFTPCPD: Trivial File Transfer Protocol Cross-Platform Daemon")
	fmt.Printf("v%v.%v.%v\n", major, minor, patch)
	fmt.Println("Usage:")
	fmt.Println("tftpcpd [options] hostname[:port]")
	fmt.Println("")
	flag.Usage()
	fmt.Println("")
	fmt.Println("If no port is specified then the daemon binds to hostname:69")
	fmt.Println("")
}

func main() {
	defer close(log)

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
		var debug *bool = flag.Bool("d", false, "enable debug mode")
		var help *bool = flag.Bool("h", false, "print usage information")
		var logFile *string = flag.String("l", "", "log file")
		var memoryLimit *int = flag.Int("M", 0, "memory limit in megabytes")

		flag.Parse()

		if *help {
			helpMessage()
			os.Exit(0)
		}

		cfg.memoryLimit = *memoryLimit
		cfg.debug = *debug
		cfg.logFile = *logFile

		args := flag.Args()
		_ = args

		if len(args) != 1 {
			helpMessage()
			os.Exit(1)
		}

		cfg.address = args[0]
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
			loggerRoutine(loggerDemandTermination, loggerConfirmTermination, log)
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
