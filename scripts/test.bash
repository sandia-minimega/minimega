#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR="$( cd ${SCRIPT_DIR}/.. && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
"$SCRIPT_DIR/version.bash" || exit $?

DIRECTORY_ARRAY=("$ROOT_DIR/cmd" "$ROOT_DIR/internal" "$ROOT_DIR/pkg")
COVERPROFILE=${COVERPROFILE:-$ROOT_DIR/coverage.out}

# Set JUNITFILE to also write a JUnit XML report.
GOTEST=(go test)
if [[ -n $JUNITFILE ]]; then
    GOTEST=(go run gotest.tools/gotestsum@v1.13.0 --format standard-quiet
        --junitfile "$JUNITFILE" --junitfile-testcase-classname relative --)
fi

PACKAGES=()
for i in "${DIRECTORY_ARRAY[@]}"; do
    for path in "$i"/*/; do
        [[ $path == */plumbing/ ]] && continue
        PACKAGES+=("$path")
    done
done
PACKAGES+=("$ROOT_DIR/tests/live/harness/")

# The harness unit tests need no root, KVM, or Docker.
PACKAGES+=("$ROOT_DIR/tests/live/harness/")

# testing
echo "TESTING"
"${GOTEST[@]}" -race -shuffle=on -coverprofile="$COVERPROFILE" "${PACKAGES[@]}" || exit 1
