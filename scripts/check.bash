#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR="$( cd ${SCRIPT_DIR}/.. && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
source $ROOT_DIR/VERSION
REVISION=`git --git-dir $ROOT_DIR/.git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Version  = \"$VERSION\"
	Revision = \"$REVISION\"
	Date     = \"$DATE\"
)" > $ROOT_DIR/internal/version/version.go

DIRECTORY_ARRAY=("$ROOT_DIR/cmd $ROOT_DIR/internal $ROOT_DIR/pkg")

echo "CHECKING FMT"
for i in ${DIRECTORY_ARRAY[@]}; do
    OUTPUT="$(find $i -type f -name '*.go' -exec gofmt -d -l {} \;)"
    if [ -n "$OUTPUT" ]; then
        echo "$OUTPUT"
        echo "gofmt - FAILED"
        exit 1
    fi
done

echo "gofmt - OK"
echo

# note: we redirect go vet's output on STDERR to STDOUT
echo "VET PACKAGES"
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -v plumbing`
    do
        echo $j
        go vet "$i/$j"
        if [[ $? != 0 ]]; then
            echo "go vet - FAILED"
            exit 1
        fi
    done
done

echo "govet - OK"
echo
