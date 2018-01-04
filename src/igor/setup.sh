#!/bin/bash -e

echo This script will run the following commands to set up igor. Press Ctrl-C to cancel, or hit Enter to continue:
echo useradd igor
echo cp ../../bin/igor /usr/local/bin/igor-bin
echo cp igor.sh /usr/local/bin/igor
echo chmod +x /usr/local/bin/igor
echo mkdir -p $1
echo mkdir -p $1/igor
echo mkdir -p $1/pxelinux.cfg/igor
echo chown igor:igor $1/igor
echo chown igor:igor $1/pxelinux.cfg/igor
echo touch $1/igor/reservations.json
echo chown igor:igor $1/igor/reservations.json
echo cp sampleconfig.json /etc/igor.conf
echo chown igor:igor /etc/igor.conf
echo chmod 600 /etc/igor.conf
echo mkdir /var/log/igor
echo chown igor:igor /var/log/igor
echo cp cronjob /etc/cron.d/igor
echo cp sudo-rule /etc/sudoers.d/igor
read
useradd igor
cp ../../bin/igor /usr/local/bin/igor-bin
cp igor.sh /usr/local/bin/igor
chmod +x /usr/local/bin/igor
mkdir -p $1
mkdir -p $1/igor
mkdir -p $1/pxelinux.cfg/igor
chown igor:igor $1/igor
chown igor:igor $1/pxelinux.cfg/igor
touch $1/igor/reservations.json
chown igor:igor $1/igor/reservations.json
cp sampleconfig.json /etc/igor.conf
chown igor:igor /etc/igor.conf
chmod 600 /etc/igor.conf
mkdir /var/log/igor
chown igor:igor /var/log/igor
cp cronjob /etc/cron.d/igor
cp sudo-rule /etc/sudoers.d/igor