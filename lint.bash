#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $SCRIPT_DIR
echo "CHECKING SOURCE CODE (golint)"
OUTPUT="$(find . ! \( -path './src/vendor' -prune \) -type f -name '*.go' -exec golint {} \;)"
LINT_EXIT=0
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "golint - FAILED"
    LINT_EXIT=1
else
	echo "golint - OK"
fi

echo
exit $LINT_EXIT