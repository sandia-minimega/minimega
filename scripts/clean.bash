#!/bin/bash

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

rm -rf $ROOT_DIR/bin
rm -f  $ROOT_DIR/doc/content/reference/minimega.md
rm -f  $ROOT_DIR/doc/content/reference/minirouter.md
rm -f  $ROOT_DIR/lib/minimega.py
rm -rf $ROOT_DIR/site
rm -f  $ROOT_DIR/internal/version/version.go
