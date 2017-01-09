#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

# set the version from the repo
VERSION=`git --git-dir $SCRIPT_DIR/.git rev-parse HEAD`
DATE=`date --rfc-3339=date`
echo "package version

var (
	Revision = \"$VERSION\"
	Date     = \"$DATE\"
)" > $SCRIPT_DIR/src/version/version.go

# build packages with race detection
#echo "BUILD RACE PACKAGES (linux)"
#echo "protonuke"
#go install -race protonuke
#mv $SCRIPT_DIR/bin/protonuke $SCRIPT_DIR/bin/protonuke_race
#echo "minimega"
#go install -race minimega
#mv $SCRIPT_DIR/bin/minimega $SCRIPT_DIR/bin/minimega_race
#echo "miniccc"
#go install -race miniccc
#mv $SCRIPT_DIR/bin/miniccc $SCRIPT_DIR/bin/miniccc_race
#echo "vmbetter"
#go install -race vmbetter
#mv $SCRIPT_DIR/bin/vmbetter $SCRIPT_DIR/bin/vmbetter_race
#echo

# build packages
echo "BUILD PACKAGES (linux)"
for i in `ls $SCRIPT_DIR/src | grep -v vendor`
do
	echo $i
	go install $i
	if [[ $? != 0 ]]; then
		exit 1
	fi
done
echo

# build windows packages
echo "BUILD PACKAGES (windows)"
echo "protonuke"
GOOS=windows go install protonuke
if [[ $? != 0 ]]; then
	exit 1
fi
echo "miniccc"
GOOS=windows go install miniccc
if [[ $? != 0 ]]; then
	exit 1
fi
echo
unset GOOS
