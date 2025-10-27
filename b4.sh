#!/bin/bash

set -eou pipefail

go test

git push backup
