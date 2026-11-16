#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..

echo BUILDING MINIMEGA...
(cd $MM && ./scripts/build.bash && ./scripts/doc.bash)
echo DONE BUILDING

# substitute version and date for changelog file
source $MM/VERSION
DATE=$(date -R)
trap 'rm -f "$SCRIPT_DIR/changelog"' EXIT
sed -e 's/${version}/'"$VERSION"'/g' -e 's/${date}/'"$DATE"'/g' $SCRIPT_DIR/changelog.base > $SCRIPT_DIR/changelog

echo BUILDING PACKAGE...


(cd $SCRIPT_DIR/.. && dpkg-buildpackage --no-sign -b)
echo DONE
