package main

import (
	"flag"
	"fmt"
	"strings"
	//"path/filepath"
	//"net"
	"os"
	"os/signal"
	//"reflect"
	"sync"
	//"time"
	//"errors"
	//"strings"
	"context"
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

	// client
	var write *string = flag.String("write", "", "tries to upload this file instead of downloading anything")

	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	internal.Cfg.Debug = *debug
	internal.Cfg.Write = *write

	args := flag.Args()

	if len(args) != 1 {
		flag.Usage()
		os.Exit(1)
	} else if internal.Cfg.Write != "" {
		// client is making a read request
		internal.Cfg.Address = args[0]
		internal.Cfg.Filename = internal.Cfg.Write
	} else {
		// client is making a write request
		splitArgs := strings.Split(args[0], "/")
		if len(splitArgs) != 2 {
			flag.Usage()
			os.Exit(1)
		}

		internal.Cfg.Address = splitArgs[0]
		internal.Cfg.Filename = splitArgs[1]
	}
}

func helpMessage(body func()) {
	major := 0
	minor := 0
	patch := 0

	fmt.Println("TFTPCPC: Trivial File Transfer Protocol Cross-Platform Client")
	fmt.Printf("v%v.%v.%v\n", major, minor, patch)
	fmt.Println("Usage:")
	fmt.Println("tftpcpc [-write filename] [options] hostname[:port][/file]")
	fmt.Println("")
	body()
	fmt.Println("")
	fmt.Println("If no port is specified then the default of port 69 is used")
	fmt.Println("")
}

func init() {
	internal.Cfg.Testing = flag.Bool("testing", false, "Used to control special behavior required for running tests.")
}

func main() {
	var (
		loggerParentToChild  chan internal.Signal = make(chan internal.Signal, 2)
		loggerChildToParent  chan internal.Signal = make(chan internal.Signal, 2)
		clientContext        context.Context
		clientCancelFunction context.CancelFunc
		interruptHandler     chan os.Signal = make(chan os.Signal, 2)
		wg                   sync.WaitGroup
		exitCode             int
	)

	//locals
	defer close(loggerParentToChild)
	defer close(loggerChildToParent)
	defer close(interruptHandler)

	processFlags()
	if internal.LoggerInit() != nil {
		os.Exit(2)
	}

	// inform user how to exit and start goroutines
	{
		// logger routine collects logs
		wg.Go(func() {
			internal.LoggerRoutine(loggerChildToParent, loggerParentToChild)
		})

		// client routine exits with 0 if it completes successfully
		clientContext, clientCancelFunction = context.WithCancel(context.Background())
		wg.Go(func() {
			options := make(map[string]string)
			var doingWrite bool = internal.Cfg.Write != ""
			internal.ClientRoutine(doingWrite, internal.Cfg.Filename, options, clientContext)
		})
	}

	// handle child goroutines terminating and signals
	{
		exitCode = 0
		signal.Notify(interruptHandler, os.Interrupt)
		select {
		case <-interruptHandler:
			clientCancelFunction()
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
				clientCancelFunction()
				close(internal.Log)
				loggerParentToChild <- internal.NewSignal(internal.SignalTerminate, internal.SignalAccept)
			} else {
				// impossible
				panic("AHHHHHHHHHHHHHHHHHHH!")
			}

			exitCode = 11
		}
	}

	// wait for all goroutines to finished
	wg.Wait()

	// Exit using code we set
	os.Exit(exitCode)
}
