# Trivial File Transfer Protocol Cross-Platform Daemon

Basic TFTP server functionality works. Current progess and future plans on roadmap listed in this document.

## Building
1. apt install golang
1. git clone https://github.com/moretiles/tftpcpd
1. cd tftpcpd
1. make prepare build

## How to use
./tftpcpd starts a tftp server listening on 127.0.0.1:8173.
Different options can be controlled through flags when starting the program.
Use the --help flag to learn about them.

## Cross Compilation
Building requires zig as github.com/mattn/go-sqlite3 forces CGO to be enabled which presents unique challenges when compiling for different operating systems.
I can promise that asking zig to build using LLVM is easier than installing 12 different versions of GCC.
Due to the licensing used for Apple's Mac OS SDK, which is required when compiling for Mac OS, you are asked to download and place this at ./MacOSX\_SDK/.
Considering that cross-platform builds are normally done as part of CI/CD, the assumption is made that the host operating system is Linux.
Additional, bespoke steps may be needed on Windows or Mac OS to acheive the same result.

## Current Progress
* Downloading files works.
* Uploading files works.
* Allow setting a root/ directory for the server to use and use OpenInRoot to prevent path traversal attacks.
* Create builds for Windows, Linux (amd64 and arm64), and Mac (amd64 and arm64).

## Todo
* Enable additional logging when using debug mode.
* Write logs to stdout/stderr by default and allow setting file to use for normal log and error log.
* Switch to more general "signal" channel for inter-goroutine coordination.
* Prevent race condition when multiple users try to upload a file named the same thing.
* Prevent race condition where out-of-date files are deleted while being read.
