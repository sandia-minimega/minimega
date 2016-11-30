#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

. $SCRIPT_DIR/addversion.bash

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
	    LINUX_PACKAGES_ERRORS=true
	fi
done
echo

# build windows packages
echo "BUILD PACKAGES (windows)"
echo "protonuke"
GOOS=windows go install protonuke
if [[ $? != 0 ]]; then
    WINDOWS_PACKAGES_ERRORS=true
fi
echo "miniccc"
GOOS=windows go install miniccc
if [[ $? != 0 ]]; then
    WINDOWS_PACKAGES_ERRORS=true
fi
echo
unset GOOS

# build python bindings
echo "BUILD PYTHON BINDINGS"
$SCRIPT_DIR/misc/python/genapi.py $SCRIPT_DIR/bin/minimega > \
    $SCRIPT_DIR/misc/python/minimega.py 2> /dev/null
if [[ $? != 0 ]]; then
    echo "minimega.py - FAILED"
	PYTHON_BINDINGS_ERRORS=true
else
    echo "minimega.py - OK"
fi
echo

if [ "$LINUX_PACKAGES_ERRORS" = "true" ] || [ "$PYTHON_BINDINGS_ERRORS" = "true" ] || [ "$WINDOWS_PACKAGES_ERRORS" = "true" ]; then
    exit 1
fi
