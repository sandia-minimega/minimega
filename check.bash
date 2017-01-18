#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $SCRIPT_DIR/src

echo "CHECKING SOURCE CODE"
OUTPUT="$(find . ! \( -path './vendor' -prune \) -type f -name '*.go' -exec gofmt -d -l {} \;)"
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "gofmt - FAILED"
    FMT_ERRORS=true
else
	echo "gofmt - OK"
fi

# note: we redirect go vet's output on STDERR to STDOUT
OUTPUT="$(find . ! \( -path './vendor' -prune \) -type f -name '*.go' -exec go vet {} 2>&1 \;)"
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "go vet - FAILED"
    VET_ERRORS=true
else
	echo "go vet - OK"
fi
echo

if [ "$FMT_ERRORS" = "true" ] | [ "$VET_ERRORS" = "true" ]; then
    exit 1
fi
