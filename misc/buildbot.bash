#!/bin/bash

# buildbot script for nightly builds on minimega.org

export PATH=${PATH}:/opt/go/bin

cd /home/fritz/buildbot/mega
git pull
./clean.bash
./all.bash > build.log
cd /home/fritz/buildbot
DATE=`date +%Y-%m-%d`
mkdir mega-$DATE && cd mega-$DATE
mkdir bin
mkdir src
mkdir misc
cp ../mega/bin/* bin
cp -r ../mega/src/* src
cp -r ../mega/misc/* misc
cd ..
tar cjf mega-${DATE}.tar.bz2 mega-$DATE
rm /home/fritz/web/minimega.org/nightly/*
cp mega-${DATE}.tar.bz2 /home/fritz/web/minimega.org/nightly
cp mega/build.log /home/fritz/web/minimega.org/nightly/mega-${DATE}.log
ln -s /home/fritz/web/minimega.org/nightly/mega-${DATE}.tar.bz2 /home/fritz/web/minimega.org/nightly/latest.tar.bz2
rm mega-${DATE}.tar.bz2
rm -rf mega-${DATE}
