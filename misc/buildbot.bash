#!/bin/bash

# buildbot script for nightly builds on minimega.org

export PATH=${PATH}:/opt/go/bin

cd /home/fritz/buildbot/minimega
git pull
./clean.bash
./build.bash > build.log
cd /home/fritz/buildbot
DATE=`date +%Y-%m-%d`
mkdir minimega-$DATE && cd minimega-$DATE
mkdir bin
mkdir src
mkdir misc
cp ../mega/bin/* bin
cp -r ../mega/src/* src
cp -r ../mega/misc/* misc
cd ..
tar cjf minimega-${DATE}.tar.bz2 minimega-$DATE
rm /home/fritz/web/minimega.org/nightly/*
cp minimega-${DATE}.tar.bz2 /home/fritz/web/minimega.org/nightly
cp minimega/build.log /home/fritz/web/minimega.org/nightly/minimega-${DATE}.log
ln -s /home/fritz/web/minimega.org/nightly/minimega-${DATE}.tar.bz2 /home/fritz/web/minimega.org/nightly/minimega-latest.tar.bz2
rm minimega-${DATE}.tar.bz2
rm -rf minimega-${DATE}
