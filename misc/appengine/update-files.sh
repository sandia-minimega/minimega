#!/bin/sh -e

# Argument 1: path to minimega source directory
if test "$#" -ne 1; then
	echo "Specify path to minimega directory as argument"
	return
fi

cp -r $1/src/minidoc/* .
cp -r $1/src/present .
cp -r $1/src/minilog .
mkdir -p golang.org/x/net
cp -r $1/src/vendor/golang.org/x/net/websocket golang.org/x/net/.
cp -r $1/src/minicli .
cp -r $1/src/ranges .
cp -r $1/doc .

echo Now run "appcfg.py --oauth2 update ."
