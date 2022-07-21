#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR="$( cd ${SCRIPT_DIR}/.. && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
VERSION=`git --git-dir $ROOT_DIR/.git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Revision = \"$VERSION\"
	Date     = \"$DATE\"
)" > $ROOT_DIR/internal/version/version.go

DIRECTORY_ARRAY=("$ROOT_DIR/cmd $ROOT_DIR/internal $ROOT_DIR/pkg")

# testing
echo "TESTING"
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -v plumbing`
    do
        go test $i/$j
        if [[ $? != 0 ]]; then
            exit 1
        fi
    done
done
