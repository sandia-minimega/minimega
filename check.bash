#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

cd $SCRIPT_DIR/src

echo "CHECKING SOURCE CODE"
OUTPUT="$(find . ! \( -path './vendor' -prune \) -type f -name '*.go' -exec gofmt -d -l {} \;)"
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "gofmt - FAILED"
    exit 1
fi

echo "gofmt - OK"
echo

# note: we redirect go vet's output on STDERR to STDOUT
echo "VET PACKAGES"
for i in `ls $SCRIPT_DIR/src | grep -v vendor | grep -v plumbing`
do
	echo $i
	go vet $i
	if [[ $? != 0 ]]; then
        echo "go vet - FAILED"
		exit 1
	fi
done

echo "govet - OK"
echo
