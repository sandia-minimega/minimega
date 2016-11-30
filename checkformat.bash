#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $SCRIPT_DIR
echo "CHECKING SOURCE CODE (gofmt)"
OUTPUT="$(find . ! \( -path './src/vendor' -prune \) -type f -name '*.go' -exec gofmt -d -l {} \;)"
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "gofmt - FAILED"
    echo
    exit 1
fi
echo "gofmt - OK"
echo
