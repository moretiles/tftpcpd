#!/bin/bash

set -eou pipefail

# run tests
make test

# build for release
make -B release

# check for vulnerabilities
govulncheck

rm -f .b4.lock
