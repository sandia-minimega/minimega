# Golang has issues adding build ids
%global _missing_build_ids_terminate_build 0
%define _build_id_links none

Name:          minimega
Version:       ${version}
Release:       1%{?dist}
Summary:       A distributed VM management tool.
License:       GPLv3

Group:         utils
URL: https://www.sandia.gov/minimega
#Source0: https://github.com/sandia-minimega/minimega.git
BuildArch:     x86_64
AutoReqProv: no

Provides:      minimega = 1%{?dist}
Provides:      minimega(x86-64) = 1%{?dist}

Requires:      /bin/bash
Requires:      /bin/sh
Requires:      /usr/bin/env
Requires:      qemu-img
Requires:      qemu-kvm
Requires:      qemu-kvm-common
Requires:      dnsmasq
Requires:      dosfstools
Requires:      net-tools
Requires:      ntfs-3g
Requires:      libpcap
Requires:      openssl-devel
Requires(post): /bin/sh



%description
minimega is a tool for launching and managing virtual machines. It can run on
your laptop or distributed across a cluster. minimega is fast, easy to deploy,
and can scale to run on massive clusters with virtually no setup.

**Note:** This package requires the EPEL repository to be enabled, as it depends on the `ntfs-3g` package, which is only available in EPEL.

%build
MM="../../../../"
(cd $MM && ./scripts/build.bash && ./scripts/doc.bash)

%install
MM="../../../../"

rm -rf $RPM_BUILD_ROOT
rm -rf %{_rpmdir}/*

mkdir -p $RPM_BUILD_ROOT/opt/minimega/misc
mkdir -p $RPM_BUILD_ROOT/usr/share/doc/minimega
mkdir -p $RPM_BUILD_ROOT/usr/bin
mkdir -p $RPM_BUILD_ROOT/etc/minimega
mkdir -p $RPM_BUILD_ROOT/usr/lib/systemd/system

cp -Lr $MM/bin $RPM_BUILD_ROOT/opt/minimega/
cp -Lr $MM/doc $RPM_BUILD_ROOT/opt/minimega/
cp -Lr $MM/lib $RPM_BUILD_ROOT/opt/minimega/
cp -Lr $MM/misc/daemon $RPM_BUILD_ROOT/opt/minimega/misc/
cp -Lr $MM/misc/vmbetter_configs $RPM_BUILD_ROOT/opt/minimega/misc/
cp -Lr $MM/web $RPM_BUILD_ROOT/opt/minimega/
cp -Lr $MM/LICENSE $RPM_BUILD_ROOT/usr/share/doc/minimega/
cp -Lr $MM/LICENSES $RPM_BUILD_ROOT/usr/share/doc/minimega/

# Make future symlinks
ln -sf /opt/minimega/bin/minimega $RPM_BUILD_ROOT/usr/bin/minimega
ln -sf /opt/minimega/bin/miniweb $RPM_BUILD_ROOT/usr/bin/miniweb
ln -sf /opt/minimega/bin/protonuke $RPM_BUILD_ROOT/usr/bin/protonuke
ln -sf /opt/minimega/misc/daemon/minimega.conf $RPM_BUILD_ROOT/etc/minimega/minimega.conf

ln -sf /opt/minimega/misc/daemon/minimega.service $RPM_BUILD_ROOT/usr/lib/systemd/system/minimega.service

find $RPM_BUILD_ROOT -type f -printf "/%%P\n" > $RPM_BUILD_ROOT/../../buildfiles

%files -f %{buildroot}/../../buildfiles
%verify(link) /etc/minimega/minimega.conf
%verify(link) /usr/bin/minimega
%verify(link) /usr/bin/miniweb
%verify(link) /usr/bin/protonuke
%verify(link) /usr/lib/systemd/system/minimega.service

%postun
if [ $1 -ge 1 ] && [ -x "/usr/lib/systemd/systemd-update-helper" ]; then
    # Package upgrade, not uninstall
    /usr/lib/systemd/systemd-update-helper mark-reload-system-units minimega.service || :
fi

%preun
if [ $1 -eq 0 ] && [ -x "/usr/lib/systemd/systemd-update-helper" ]; then
    # Package removal, not upgrade
    /usr/lib/systemd/systemd-update-helper remove-system-units minimega.service || :
fi

if [ $1 -eq 0 ]; then
    /usr/bin/systemctl --no-reload disable minimega.service
    /usr/bin/systemctl stop minimega.service >/dev/null 2>&1 ||:
    /usr/bin/systemctl disable minimega.service

fi
if [ $1 -eq 1 ]; then
    /usr/bin/systemctl --no-reload disable minimega.service
    /usr/bin/systemctl stop %minimega.service
fi

%post -p /bin/sh
#! /bin/sh

set -e

if ! id -u minimega >/dev/null 2>&1; then
    echo "Adding minimega user."
    useradd --system --no-create-home --home-dir /run/minimega minimega
else
    echo "minimega user already exists."
fi

MINIMEGA_BIN="/usr/bin/minimega"
LIBPCAP_SO_0_8="/usr/lib64/libpcap.so.0.8"
LIBPCAP_SO="/usr/lib64/libpcap.so"
LIBPCAP_SO_1="/usr/lib64/libpcap.so.1"

if [ -f "$MINIMEGA_BIN" ]; then
    echo "Checking if $MINIMEGA_BIN links to libpcap..."

    # Check the linked libraries
    LINKED_LIBS=$(ldd "$MINIMEGA_BIN" | grep "libpcap.so.0.8")

    if echo "$LINKED_LIBS" | grep -q "not found"; then
        echo "Warning: $MINIMEGA_BIN links to libpcap.so.0.8, which is not found."

        # First, check if libpcap.so exists
        if [ -f "$LIBPCAP_SO" ]; then
            echo "Creating symbolic link for libpcap.so.0.8 to libpcap.so"
            ln -sf "$LIBPCAP_SO" "$LIBPCAP_SO_0_8"
            echo "Symbolic link created: $LIBPCAP_SO_0_8 -> $LIBPCAP_SO"
        # If libpcap.so doesn't exist, check for libpcap.so.1
        elif [ -f "$LIBPCAP_SO_1" ]; then
            echo "Creating symbolic link for libpcap.so.0.8 to libpcap.so.1"
            ln -sf "$LIBPCAP_SO_1" "$LIBPCAP_SO_0_8"
            echo "Symbolic link created: $LIBPCAP_SO_0_8 -> $LIBPCAP_SO_1"
        else
            echo "Error: Neither libpcap.so nor libpcap.so.1 is installed. Cannot create the symbolic link."
            exit 1
        fi
    fi
else
    echo "Error: $MINIMEGA_BIN does not exist."
    exit 1
fi

chown -R minimega:minimega /usr/share/doc/minimega
chown -R minimega:minimega /opt/minimega
chown -R minimega:minimega /etc/minimega

if [ $1 -eq 1 ]; then
    /usr/bin/systemctl daemon-reload
    /usr/bin/systemctl start minimega.service
fi
if [ $1 -eq 2 ]; then
    /usr/bin/systemctl daemon-reload
    /usr/bin/systemctl start minimega.service
fi

if [ $1 -eq 1 ] && [ -x "/usr/lib/systemd/systemd-update-helper" ]; then
    # Initial installation
    /usr/lib/systemd/systemd-update-helper install-system-units minimega.service || :
fi

%clean
rm $RPM_BUILD_ROOT/../../buildfiles
rm -rf $RPM_BUILD_ROOT
