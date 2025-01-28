#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..
FILES=$SCRIPT_DIR/minmega

# substitute version for control file
source $MM/VERSION
sed -e 's/${version}/'"$VERSION"'/g' $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec.base > $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec

echo "PREPARING FOR BUILDING RPM..."
mkdir -p rpmbuild/BUILD
mkdir -p rpmbuild/RPMS
mkdir -p rpmbuild/SOURCES
mkdir -p rpmbuild/SPECS
mkdir -p rpmbuild/SRPMS

echo "BUILDING PACKAGE..."
(cd $SCRIPT_DIR && rpmbuild --define "_topdir $SCRIPT_DIR/rpmbuild" -bb rpmbuild/SPECS/minimega.spec)
cp $SCRIPT_DIR/rpmbuild/RPMS/x86_64/*.rpm $SCRIPT_DIR/../../
rm -rf $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec
echo "DONE"
