#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $SCRIPT_DIR
echo "CHECKING SOURCE CODE (go vet)"
# note: we redirect go vet's output on STDERR to STDOUT
OUTPUT="$(find . ! \( -path './src/vendor' -prune \) -type f -name '*.go' -exec go vet {} 2>&1 \;)"
VET_EXIT=0
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "go vet - FAILED"
    echo
    exit 1
fi
echo "go vet - OK"
echo
