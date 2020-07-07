#!/bin/bash

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

rm -rf $ROOT_DIR/bin
rm -f  $ROOT_DIR/doc/markdown/api
rm -f  $ROOT_DIR/doc/*.html
rm -f  $ROOT_DIR/internal/version/version.go
