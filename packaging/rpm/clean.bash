#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

rm -rf "$SCRIPT_DIR/rpmbuild/BUILD"
rm -rf "$SCRIPT_DIR/rpmbuild/BUILDROOT"
rm -rf "$SCRIPT_DIR/rpmbuild/RPMS"
rm -rf "$SCRIPT_DIR/rpmbuild/SOURCES"
rm -rf "$SCRIPT_DIR/rpmbuild/SRPMS"
