#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source $SCRIPT_DIR/env.bash

# set the version from the repo
VERSION=`git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Revision = \"$VERSION\"
	Date = \"$DATE\"
)
" > $SCRIPT_DIR/src/version/version.go

# build packages
echo "BUILD PACKAGES (linux)"
for i in `ls $SCRIPT_DIR/src`
do
	echo $i
	go install $i
done
echo

echo "BUILD PACKAGES (windows)"
echo "protonuke"
go install protonuke
echo "miniccc"
go install miniccc
echo

# testing 
echo TESTING
for i in `ls $SCRIPT_DIR/src`
do
	go test $i
done
echo
