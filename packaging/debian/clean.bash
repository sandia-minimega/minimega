#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

rm -rf $SCRIPT_DIR/minimega
rm -rf $SCRIPT_DIR/*.debhelper*
rm -rf $SCRIPT_DIR/minimega.service
rm -rf $SCRIPT_DIR/minimega.substvars
rm -rf $SCRIPT_DIR/debhelper-build-stamp
rm -rf $SCRIPT_DIR/tmp
rm -f $SCRIPT_DIR/changelog
