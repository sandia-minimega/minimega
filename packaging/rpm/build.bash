#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..

# substitute version and date for spec file
source $MM/VERSION
DATE=$(LC_ALL=C date +'%a %b %d %Y')
trap 'rm -f "$SCRIPT_DIR/rpmbuild/SPECS/minimega.spec"' EXIT
sed -e 's/${version}/'"$VERSION"'/g' -e 's/${date}/'"$DATE"'/g' $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec.base > $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec

echo "PREPARING FOR BUILDING RPM..."
mkdir -p $SCRIPT_DIR/rpmbuild/BUILD
mkdir -p $SCRIPT_DIR/rpmbuild/RPMS
mkdir -p $SCRIPT_DIR/rpmbuild/SOURCES
mkdir -p $SCRIPT_DIR/rpmbuild/SPECS
mkdir -p $SCRIPT_DIR/rpmbuild/SRPMS

echo "BUILDING PACKAGE..."
(cd $SCRIPT_DIR && rpmbuild --define "_topdir $SCRIPT_DIR/rpmbuild" -bb rpmbuild/SPECS/minimega.spec)
cp $SCRIPT_DIR/rpmbuild/RPMS/x86_64/*.rpm $SCRIPT_DIR/../../
echo "DONE"
