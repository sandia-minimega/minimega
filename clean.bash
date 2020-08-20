#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

rm -rf $SCRIPT_DIR/bin
rm -rf $SCRIPT_DIR/pkg
rm -f $SCRIPT_DIR/doc/markdown/api
rm -f $SCRIPT_DIR/doc/*.html
rm -f $SCRIPT_DIR/src/version/version.go
