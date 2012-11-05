#!/bin/sh -e

echo This script will run the following commands to set up igor. Press Ctrl-C to cancel, or hit Enter to continue:
echo mkdir $1/igor
echo mkdir $1/pxelinux.cfg/igor
echo touch $1/igor/reservations.json
echo cp sampleconfig.json /etc/igor.conf
read
mkdir $1/igor
mkdir $1/pxelinux.cfg/igor
touch $1/igor/reservations.json
cp sampleconfig.json /etc/igor.conf
