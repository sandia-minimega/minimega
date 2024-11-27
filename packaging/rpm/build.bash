#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..
FILES=$SCRIPT_DIR/minmega

echo "BUILDING MINIMEGA..."
(cd $MM && ./scripts/build.bash && ./scripts/doc.bash)
echo "DONE BUILDING"


# substitute version for control file
source $MM/VERSION
sed -i -e 's/${version}/'"$VERSION"'/g' $SCRIPT_DIR/rpmbuild/SPECS/minimega.spec

echo "PREPARING FOR BUILDING RPM..."
mkdir -p rpmbuild/BUILD
mkdir -p rpmbuild/RPMS
mkdir -p rpmbuild/SOURCES
mkdir -p rpmbuild/SPECS
mkdir -p rpmbuild/SRPMS

echo "BUILDING PACKAGE..."
(cd $SCRIPT_DIR && rpmbuild --define "_topdir $SCRIPT_DIR/rpmbuild" -bb rpmbuild/SPECS/minimega.spec)
echo "DONE"
