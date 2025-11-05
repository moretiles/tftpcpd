#!/bin/bash

set -eou pipefail

# run tests
make test

rm -f .b4.lock
