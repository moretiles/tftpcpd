#!/bin/bash

set -eou pipefail

# run unit tests
make test

# run integration tests
./test.sh

# build for release
make -B release

# check for vulnerabilities
govulncheck

rm -f .b4.lock
