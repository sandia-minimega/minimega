#!/bin/sh -e
# You *must* run this from the packaging/debian directory.

mm_root=`pwd`/../../
dest_opt=./minimega/opt/minimega/

echo BUILDING MINIMEGA...
(cd $mm_root && ./build.bash)
echo DONE BUILDING

echo COPYING FILES...
mkdir -p ./minimega/opt/minimega/
cp -r $mm_root/bin $dest_opt
cp -r $mm_root/misc $dest_opt
cp -r $mm_root/doc $dest_opt

mkdir -p ./minimega/usr/share/doc/minimega
cp $mm_root/LICENSES/* ./minimega/usr/share/doc/minimega/
echo COPIED FILES

echo BUILDING PACKAGE...
fakeroot dpkg-deb -b minimega
echo DONE
