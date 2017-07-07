#!/bin/bash

# build a busybox based, minimal minirouter rootfs
mkdir -p minirouterfs/dev
mkdir -p minirouterfs/bin
mkdir -p minirouterfs/sbin
mkdir -p minirouterfs/usr/bin
mkdir -p minirouterfs/usr/sbin
mkdir -p minirouterfs/var/run
mkdir -p minirouterfs/var/lib/dhcp
mkdir -p minirouterfs/var/lib/misc
mkdir -p minirouterfs/tmp
mkdir -p minirouterfs/proc
mkdir -p minirouterfs/sys
mkdir -p minirouterfs/etc
mkdir -p minirouterfs/root
mkdir -p minirouterfs/lib/x86_64-linux-gnu

wget https://busybox.net/downloads/binaries/busybox-x86_64
mv busybox-x86_64 minirouterfs/bin/busybox
chmod a+rx minirouterfs/bin/busybox

cd minirouterfs
for i in `bin/busybox --list-full`
do
	ln -s /bin/busybox $i
done
cd ..

PATH=${PATH}:/usr/sbin:/sbin
rm minirouterfs/sbin/ip
cp init minirouterfs
cp preinit minirouterfs
cp ../../bin/minirouter minirouterfs
cp ../../bin/miniccc minirouterfs
cp `which dhclient` minirouterfs/sbin
cp `which ip` minirouterfs/sbin
cp `which dnsmasq` minirouterfs/usr/sbin
cp `which bird` minirouterfs/usr/sbin
cp `which bird6` minirouterfs/usr/sbin

go run ../ldd.go minirouterfs/miniccc minirouterfs
go run ../ldd.go minirouterfs/minirouter minirouterfs
go run ../ldd.go minirouterfs/sbin/ip minirouterfs
go run ../ldd.go minirouterfs/usr/sbin/dnsmasq minirouterfs
go run ../ldd.go minirouterfs/usr/sbin/bird minirouterfs
go run ../ldd.go minirouterfs/usr/sbin/bird6 minirouterfs

# scripts that dhclient depends on
cp `which dhclient-script` minirouterfs/sbin
mkdir -p minirouterfs/etc/dhcp/dhclient-enter-hooks.d
mkdir -p minirouterfs/etc/dhcp/dhclient-exit-hooks.d
cp /etc/dhcp/dhclient.conf minirouterfs/etc/dhcp
cp /etc/dhcp/dhclient-exit-hooks.d/rfc3442-classless-routes minirouterfs/etc/dhcp/dhclient-exit-hooks.d

# because busybox doesn't have bash
sed -i 's/bash/sh/' minirouterfs/sbin/dhclient-script

# libc dlopen's libnss directly, so ldd won't grab it for us
cp  /lib/x86_64-linux-gnu/libnss* minirouterfs/lib/x86_64-linux-gnu/

echo "root:x:0:0:root:/root:/bin/sh" > minirouterfs/etc/passwd

tar czf minirouter.tar.gz minirouterfs
