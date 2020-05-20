#!/bin/bash

# build a busybox based, minimal minirouter rootfs
mkdir -p uminirouterfs/dev
mkdir -p uminirouterfs/bin
mkdir -p uminirouterfs/sbin
mkdir -p uminirouterfs/usr/bin
mkdir -p uminirouterfs/usr/sbin
mkdir -p uminirouterfs/var/run
mkdir -p uminirouterfs/var/lib/dhcp
mkdir -p uminirouterfs/var/lib/misc
mkdir -p uminirouterfs/tmp
mkdir -p uminirouterfs/proc
mkdir -p uminirouterfs/sys
mkdir -p uminirouterfs/etc
mkdir -p uminirouterfs/root
mkdir -p uminirouterfs/lib/x86_64-linux-gnu

wget https://busybox.net/downloads/binaries/1.21.1/busybox-x86_64
mv busybox-x86_64 uminirouterfs/bin/busybox
chmod a+rx uminirouterfs/bin/busybox

cd uminirouterfs
for i in `bin/busybox --list-full`
do
	ln -s /bin/busybox $i
done
cd ..

PATH=${PATH}:/usr/sbin:/sbin
rm uminirouterfs/sbin/ip
cp init uminirouterfs
cp preinit uminirouterfs
cp ../../bin/minirouter uminirouterfs
cp ../../bin/miniccc uminirouterfs
cp `which dhclient` uminirouterfs/sbin
cp `which ip` uminirouterfs/sbin
cp `which dnsmasq` uminirouterfs/usr/sbin
cp `which bird` uminirouterfs/usr/sbin
cp `which bird6` uminirouterfs/usr/sbin

go run ../ldd.go uminirouterfs/miniccc uminirouterfs
go run ../ldd.go uminirouterfs/minirouter uminirouterfs
go run ../ldd.go uminirouterfs/sbin/dhclient uminirouterfs
go run ../ldd.go uminirouterfs/sbin/ip uminirouterfs
go run ../ldd.go uminirouterfs/usr/sbin/dnsmasq uminirouterfs
go run ../ldd.go uminirouterfs/usr/sbin/bird uminirouterfs
go run ../ldd.go uminirouterfs/usr/sbin/bird6 uminirouterfs

# scripts that dhclient depends on
cp `which dhclient-script` uminirouterfs/sbin
mkdir -p uminirouterfs/etc/dhcp/dhclient-enter-hooks.d
mkdir -p uminirouterfs/etc/dhcp/dhclient-exit-hooks.d
cp /etc/dhcp/dhclient.conf uminirouterfs/etc/dhcp
cp /etc/dhcp/dhclient-exit-hooks.d/rfc3442-classless-routes uminirouterfs/etc/dhcp/dhclient-exit-hooks.d

# because busybox doesn't have bash
sed -i 's/bash/sh/' uminirouterfs/sbin/dhclient-script

# libc dlopen's libnss directly, so ldd won't grab it for us
cp  /lib/x86_64-linux-gnu/libnss* uminirouterfs/lib/x86_64-linux-gnu/

echo "root:x:0:0:root:/root:/bin/sh" > uminirouterfs/etc/passwd
echo "# UNCONFIGURED FSTAB FOR BASE SYSTEM" > uminirouterfs/etc/fstab

tar czf uminirouterfs.tar.gz uminirouterfs
