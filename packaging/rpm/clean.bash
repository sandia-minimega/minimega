#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

rm -rf rpmbuild/BUILD
rm -rf rpmbuild/BUILDROOT
rm -rf rpmbuild/RPMS
rm -rf rpmbuild/SOURCES
rm -rf rpmbuild/SRPMS
