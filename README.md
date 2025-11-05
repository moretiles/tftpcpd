# Trivial File Transfer Protocol Cross-Platform Daemon

## Current Progress
* Downloading files works.
* Uploading files works.
* Allow setting a root/ directory for the server to use and use OpenInRoot to prevent path traversal attacks.

## Todo
* Enable additional logging when using debug mode.
* Write logs to stdout/stderr by default and allow setting file to use for normal log and error log.
* Switch to more general "signal" channel for inter-goroutine coordination.
* Prevent race condition when multiple users try to upload a file named the same thing.
* Prevent race condition where out-of-date files are deleted while being read.
