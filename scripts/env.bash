# shellcheck shell=bash
# Source this file; do not run it.
GOBIN="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )/bin"
export GOBIN
export GOFLAGS=-mod=vendor
