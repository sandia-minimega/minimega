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

DIRECTORY_ARRAY=("$SCRIPT_DIR/cmd $SCRIPT_DIR/cmd/plumbing $SCRIPT_DIR/internal $SCRIPT_DIR/pkg")

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
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -v vendor | grep -v plumbing`
    do
        echo $j
        go install $i/$j
        if [[ $? != 0 ]]; then
            exit 1
        fi
    done
done

# build windows packages
echo "BUILD PACKAGES (windows)"
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -E "protonuke|miniccc"`; do
        echo $j
        GOOS=windows go build -o $SCRIPT_DIR/bin/$j.exe $i/$j
        if [[ $? != 0 ]]; then
            exit 1
        fi
    done
done
echo

unset GOOS
