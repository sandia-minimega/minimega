#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
VERSION=`git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Revision = \"$VERSION\"
	Date = \"$DATE\"
)
" > $SCRIPT_DIR/src/version/version.go

# testing
echo TESTING
for i in `ls $SCRIPT_DIR/src`
do
	go test $i
done
echo
