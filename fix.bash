#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $SCRIPT_DIR
echo "CHECKING SOURCE CODE (go tool fix)"
OUTPUT="$(find . ! \( -path './src/vendor' -prune \) -type f -name '*.go' -exec go tool fix -diff {} 2>&1 \;)"
FIX_EXIT=0
if [ -n "$OUTPUT" ]; then
    echo "$OUTPUT"
    echo "go tool fix - FAILED"
    FIX_EXIT=1
else
	echo "go tool fix - OK"
fi

echo
exit $s