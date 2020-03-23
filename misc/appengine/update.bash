#!/bin/bash

# stop on error

URL=https://github.com/sandia-minimega/minimega.git
ROOT=/home/cjs/go

REPO=$ROOT/src/minimega
TARGET=$ROOT/src/mmorg
SDK=$ROOT/src/google-cloud-sdk

PATH=$PATH:/usr/local/go/bin

#if [ -d $REPO ]; then
#    rm -rf $REPO
#fi
#
#git clone $URL $REPO
#(cd $REPO && bash all.bash)
#
#if [ -d $TARGET ]; then
#    rm -rf $TARGET
#fi

#mkdir $TARGET

# copy appengine
#cp $REPO/misc/appengine/app.yaml $TARGET
#cp $REPO/src/minidoc/* $TARGET
#cp -r $REPO/doc $TARGET

# copy dependencies (used to be in $TARGET/src/)
#cp -r $REPO/src/minicli $TARGET
#cp -r $REPO/src/minilog $TARGET
#cp -r $REPO/src/present $TARGET
#cp -r $REPO/src/ranges $TARGET
#mkdir -p $TARGET/golang.org/x/net
#cp -r $REPO/src/vendor/golang.org/x/net/websocket $TARGET/golang.org/x/net/

# update tip.minimega.org
#cd $TARGET/app
cd $TARGET
GOPATH=$ROOT $SDK/bin/gcloud app deploy --verbosity=debug --project pivotal-sonar-90317 --version 1

# update minimega.org
#GOPATH=$TARGET $SDK/bin/gcloud app deploy --quiet --project even-electron-88116 --version 1
