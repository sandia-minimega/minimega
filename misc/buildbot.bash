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
cp -r ../minimega/bin/* bin
cp -r ../minimega/src/* src
cp -r ../minimega/misc/* misc
cd ..
tar cjf minimega-${DATE}.tar.bz2 minimega-$DATE
rm /home/fritz/web/minimega.org/nightly/*
cp minimega-${DATE}.tar.bz2 /home/fritz/web/minimega.org/nightly
cp minimega/build.log /home/fritz/web/minimega.org/nightly/minimega-${DATE}.log
rm minimega-${DATE}.tar.bz2
rm -rf minimega-${DATE}
