#!/bin/bash

# build a busybox based, minimal miniccc rootfs
mkdir -p uminicccfs/dev
mkdir -p uminicccfs/bin
mkdir -p uminicccfs/sbin
mkdir -p uminicccfs/usr/bin
mkdir -p uminicccfs/usr/sbin
mkdir -p uminicccfs/var/run
mkdir -p uminicccfs/var/lib/dhcp
mkdir -p uminicccfs/var/lib/misc
mkdir -p uminicccfs/tmp
mkdir -p uminicccfs/proc
mkdir -p uminicccfs/sys
mkdir -p uminicccfs/etc
mkdir -p uminicccfs/root
mkdir -p uminicccfs/lib/x86_64-linux-gnu

touch uminicccfs/etc/fstab

wget https://busybox.net/downloads/binaries/1.21.1/busybox-x86_64
#https://busybox.net/downloads/binaries/busybox-x86_64
mv busybox-x86_64 uminicccfs/bin/busybox
chmod a+rx uminicccfs/bin/busybox

cd uminicccfs
for i in `bin/busybox --list-full`
do
	ln -s /bin/busybox $i
done
cd ..

PATH=${PATH}:/usr/sbin:/sbin
rm uminicccfs/sbin/ip
cp init uminicccfs
cp ../../bin/miniccc uminicccfs
cp `which dhclient` uminicccfs/sbin
cp `which ip` uminicccfs/sbin

go run ../ldd.go uminicccfs/miniccc uminicccfs
go run ../ldd.go uminicccfs/sbin/dhclient uminicccfs
go run ../ldd.go uminicccfs/sbin/ip uminicccfs

# scripts that dhclient depends on
cp `which dhclient-script` uminicccfs/sbin
mkdir -p uminicccfs/etc/dhcp/dhclient-enter-hooks.d
mkdir -p uminicccfs/etc/dhcp/dhclient-exit-hooks.d
cp /etc/dhcp/dhclient.conf uminicccfs/etc/dhcp
cp /etc/dhcp/dhclient-exit-hooks.d/rfc3442-classless-routes uminicccfs/etc/dhcp/dhclient-exit-hooks.d

# because busybox doesn't have bash
sed -i 's/bash/sh/' uminicccfs/sbin/dhclient-script

# libc dlopen's libnss directly, so ldd won't grab it for us
cp  /lib/x86_64-linux-gnu/libnss* uminicccfs/lib/x86_64-linux-gnu/

echo "root:x:0:0:root:/root:/bin/sh" > uminicccfs/etc/passwd
echo "# UNCONFIGURED FSTAB FOR BASE SYSTEM" > uminicccfs/etc/fstab

tar czf uminicccfs.tar.gz uminicccfs
