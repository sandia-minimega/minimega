#!/bin/bash

# stop on error

URL=https://github.com/sandia-minimega/minimega/v2.git
ROOT=$(go env GOPATH)
SDKFILE=google-cloud-sdk-299.0.0-linux-x86_64.tar.gz
SDKURL=https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/$SDKFILE

if [ ! -d $ROOT/src ]; then
    mkdir -p $ROOT/src
fi

REPO=$ROOT/src/minimega
TARGET=$ROOT/src/mmorg
SDK=$ROOT/src/google-cloud-sdk
GOMOD=$ROOT/src/go.mod

PATH=$PATH:/usr/local/go/bin

# If this has been run before and there are leftover files, the build will not work
if [ -d $REPO ]; then
    rm -rf $REPO
fi

if [ -d $TARGET ]; then
    rm -rf $TARGET
fi

if [ -d $SDK ]; then
    rm -rf $SDK
fi

if [ -f $GOMOD ]; then
    rm $GOMOD
fi

git clone $URL $REPO
(cd $REPO && bash all.bash)

mkdir $TARGET

# Get Google cloud sdk
wget -P $ROOT/src $SDKURL
tar -xvf $ROOT/src/$SDKFILE -C $ROOT/src

# copy appengine
cp $REPO/misc/appengine/app.yaml $TARGET
cp $REPO/misc/appengine/go.mod $TARGET
cp $REPO/src/minidoc/* $TARGET
cp -r $REPO/doc $TARGET

# copy dependencies
cp -r $REPO/src/minicli $TARGET
cp -r $REPO/src/minilog $TARGET
cp -r $REPO/src/present $TARGET
cp -r $REPO/src/ranges $TARGET
mkdir -p $TARGET/golang.org/x/net
cp -r $REPO/src/vendor/golang.org/x/net/websocket $TARGET/golang.org/x/net/

# Update include paths
# We need to do this because of the migration to go 112+ requires a specific directory structure
# This structure is not supported by our structure in minimega, so this is a hack to make it work
grep -rl \"minicli\" --exclude-dir minicli --exclude-dir doc $TARGET | xargs sed -i 's/\"minicli\"/\"mmorg\/minicli\"/'
grep -rl \"minilog\" --exclude-dir minilog $TARGET | xargs sed -i 's/\"minilog\"/\"mmorg\/minilog\"/'
grep -rl \"present\" --exclude-dir present $TARGET | xargs sed -i -e '0,/\"present\"/ s/\"present\"/\"mmorg\/present\"/'
grep -rl \"ranges\" --exclude-dir ranges $TARGET | xargs sed -i 's/\"ranges\"/\"mmorg\/ranges\"/'
sed -i 's/\"golang\.org/\"mmorg\/golang\.org/' $TARGET/socket.go


# update tip.minimega.org
cd $TARGET
GOPATH=$ROOT $SDK/bin/gcloud app deploy --verbosity=debug --project pivotal-sonar-90317 --version 1

# update minimega.org
#GOPATH=$ROOT $SDK/bin/gcloud app deploy --verbosity=debug --project even-electron-88116 --version 1
