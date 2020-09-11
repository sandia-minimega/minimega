#!/bin/bash

MODULE="github.com/sandia-minimega/minimega"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
VERSION=`git --git-dir $SCRIPT_DIR/.git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Revision = \"$VERSION\"
	Date     = \"$DATE\"
)" > $SCRIPT_DIR/internal/version/version.go

DIRECTORY_ARRAY=("$SCRIPT_DIR/cmd $SCRIPT_DIR/internal $SCRIPT_DIR/pkg")

# testing
echo "TESTING"
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -v vendor | grep -v plumbing`
    do
        go test $i/$j
        if [[ $? != 0 ]]; then
            exit 1
        fi
    done
done
