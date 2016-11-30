#!/bin/bash

OUTPUT="$(find . ! \( -path './src/vendor' -prune \) -type f -name '*.go' -exec gofmt -d -l {} \;)"
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "gofmt - FAIL"
    exit 1
fi
echo
