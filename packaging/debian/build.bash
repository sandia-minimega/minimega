#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

MM=$SCRIPT_DIR/../..

echo BUILDING MINIMEGA...
(cd $MM && ./scripts/build.bash && ./scripts/doc.bash)
echo DONE BUILDING

echo COPYING FILES...

DST=$SCRIPT_DIR/minimega/opt/minimega
mkdir -p $DST
cp -r $MM/bin $DST/
cp -r $MM/doc $DST/
cp -r $MM/lib $DST/
mkdir -p $DST/misc
cp -r $MM/misc/daemon $DST/misc/
cp -r $MM/misc/vmbetter_configs $DST/misc/
mkdir -p $DST/web
cp -r $MM/web $DST/web/

DOCS=$SCRIPT_DIR/minimega/usr/share/doc/minimega
mkdir -p $DOCS
cp $MM/LICENSE $DOCS/
cp -r $MM/LICENSES $DOCS/

echo COPIED FILES

echo BUILDING PACKAGE...
(cd $SCRIPT_DIR && fakeroot dpkg-deb -b minimega)
echo DONE
