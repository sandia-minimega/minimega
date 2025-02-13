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

DIRECTORY_ARRAY=("$ROOT_DIR/cmd $ROOT_DIR/cmd/plumbing")

# build packages with race detection
#echo "BUILD RACE PACKAGES (linux)"
#echo "protonuke"
#go install -race protonuke
#mv $ROOT_DIR/bin/protonuke $ROOT_DIR/bin/protonuke_race
#echo "minimega"
#go install -race minimega
#mv $ROOT_DIR/bin/minimega $ROOT_DIR/bin/minimega_race
#echo "miniccc"
#go install -race miniccc
#mv $ROOT_DIR/bin/miniccc $ROOT_DIR/bin/miniccc_race
#echo "vmbetter"
#go install -race vmbetter
#mv $ROOT_DIR/bin/vmbetter $ROOT_DIR/bin/vmbetter_race
#echo

# build packages
echo "BUILD PACKAGES (linux)"
for i in ${DIRECTORY_ARRAY[@]}; do
    for j in `ls $i | grep -v plumbing`; do
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
        GOOS=windows go build -o $ROOT_DIR/bin/$j.exe $i/$j
        if [[ $? != 0 ]]; then
            exit 1
        fi
    done
done
echo

# Build Python bindings
$ROOT_DIR/bin/pyapigen -out $ROOT_DIR/lib/minimega.py $ROOT_DIR/bin/minimega

# If python is installed, build a source distribution for the
# minimega Python bindings.
py_path=$(command -v python3)
if [ -z "$py_path" ]; then
    py_path=$(command -v python)
fi
if [ ! -z "$py_path" ]; then
    echo "BUILD PYTHON DIST"
    cp $ROOT_DIR/README.md $ROOT_DIR/lib/
    cp $ROOT_DIR/VERSION $ROOT_DIR/lib/
    pushd $ROOT_DIR/lib
    $py_path setup.py --quiet sdist
    popd
    rm $ROOT_DIR/lib/README.md
    rm $ROOT_DIR/lib/VERSION
fi

unset GOOS
