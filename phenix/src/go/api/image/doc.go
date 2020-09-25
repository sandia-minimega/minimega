/*
Implementation of the phenix Image API.

To build a Kali release on a non-Kali (but still Debian-based) operating
system, the following steps must be taken to prepare the host (Debian-based)
OS first. They are based on the official Kali documentation located at
https://www.kali.org/tutorials/build-kali-with-live-build-on-debian-based-systems/.

1. Download and install the latest version of the Kali archive keyring
package. At time of writing, the latest version was 2020.2.

	$> wget http://http.kali.org/kali/pool/main/k/kali-archive-keyring/kali-archive-keyring_2020.2_all.deb
	$> sudo dpkg -i kali-archive-keyring_2020.2_all.deb

2. Next, create the `debootstrap` build script for Kali, based entirely off
the existing Debian Sid build script. Note that the following commands will
likely need to be run as root.

	$> cd /usr/share/debootstrap/scripts
	$> sed -e "s/debian-archive-keyring.gpg/kali-archive-keyring.gpg/g" sid > kali
	$> ln -s kali kali-rolling

At this point, you should be able to build a Kali release with `phenix image`.
*/
package image
