#!/bin/bash

set -eou pipefail

# check for vulnerabilities
govulncheck

# run tests
make test

# build for release
make -B release

rm -f .b4.lock
