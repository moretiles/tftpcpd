#!/bin/bash

set -eou pipefail

# run tests
go test

rm -f .b4.lock
