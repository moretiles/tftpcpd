# Trivial File Transfer Protocol Cross-Platform Daemon

Basic TFTP server functionality works. Current progress and future plans on roadmap listed in this document.

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
* The above build steps should work fine. It is only if you want to build for other operating systems or architectures you should worry about this.
* Building requires zig as github.com/mattn/go-sqlite3 forces CGO to be enabled which presents unique challenges when compiling for different operating systems. Asking zig to build using LLVM is easier than installing 12 different versions of GCC.
* Due to the licensing used for Apple's Mac OS SDK, which is required when compiling for Mac OS, you are asked to download and place this at ./MacOSX\_SDK/.
* As cross-platform builds are normally done as part of CI/CD, the assumption is made that the host operating system is Linux. Additional, bespoke steps may be needed on Windows or Mac OS to achieve the same result.

## Current Progress
* Uploading and downloading files works.
* Allow setting a root/ directory for the server to use and use OpenInRoot to prevent path traversal attacks.
* Create builds for Windows, Linux (amd64 and arm64), and Mac (amd64 and arm64).
* Enable additional logging when using debug mode.
* Use SQLite to store persistent file information, preventing race conditions and removing unused out-of-date files periodically.
* Create simple TFTP client to use in testing. Plan will be to run parallel tests using my TFTP client and curl so I can know whether the client or server is at fault.

## Todo
* Clean up client code.
* Standardize errors.
