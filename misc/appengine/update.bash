#!/bin/bash

# stop on error
set -e

URL=https://github.com/sandia-minimega/minimega.git
ROOT=/data/appengine

REPO=$ROOT/minimega
TARGET=$ROOT/deploy
SDK=$ROOT/google-cloud-sdk

PATH=$PATH:/usr/local/go/bin

if [ -d $REPO ]; then
    rm -rf $REPO
fi

git clone $URL $REPO
(cd $REPO && bash all.bash)

if [ -d $TARGET ]; then
    rm -rf $TARGET
fi

mkdir -p $TARGET/app
mkdir -p $TARGET/src

# copy appengine
cp $REPO/misc/appengine/app.yaml $TARGET/app/
cp $REPO/src/minidoc/* $TARGET/app/
cp -r $REPO/doc $TARGET/app/

# copy dependencies
cp -r $REPO/src/minicli $TARGET/src/
cp -r $REPO/src/minilog $TARGET/src/
cp -r $REPO/src/present $TARGET/src/
cp -r $REPO/src/ranges $TARGET/src/
mkdir -p $TARGET/src/golang.org/x/net
cp -r $REPO/src/vendor/golang.org/x/net/websocket $TARGET/src/golang.org/x/net/

# update tip.minimega.org
cd $TARGET/app
GOPATH=$TARGET $SDK/bin/gcloud app deploy --quiet --project pivotal-sonar-90317 --version 1

# update minimega.org
#GOPATH=$TARGET $SDK/bin/gcloud app deploy --quiet --project even-electron-88116 --version 1
