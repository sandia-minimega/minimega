#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..

echo BUILDING MINIMEGA...
(cd $MM && ./scripts/build.bash && ./scripts/doc.bash)
echo DONE BUILDING

# substitute version for control file
source $MM/VERSION
DATE=$(date -R)
sed -e 's/${version}/'"$VERSION"'/g' changelog.base > changelog.out
sed -e 's/${date}/'"$DATE"'/g' changelog.out > changelog
rm changelog.out
cp $MM/misc/daemon/minimega.service .

echo BUILDING PACKAGE...


(cd $SCRIPT_DIR/.. && dpkg-buildpackage --no-sign -b)
rm $SCRIPT_DIR/changelog
echo DONE

