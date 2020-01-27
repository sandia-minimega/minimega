# Wrapper script to simplify building vmbetter images.
#
# Example:
#
#   bash vmbetter.bash ccc_host carnac_host ccc_host_ubuntu carnac_host_ubuntu

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT=$SCRIPT_DIR/../

mirror="http://ftp.us.debian.org/debian/"
branch=stable
level=info

vmbetter() {
	$ROOT/bin/vmbetter -mirror=$mirror -branch=$branch -level=$level $@
}

vmbetter_ubuntu() {
	local mirror=http://us.archive.ubuntu.com/ubuntu/
	local branch=xenial

	vmbetter -constraints=ubuntu -debootstrap-append "--components=main,universe,restricted,multiverse" $@
}

for i in "$@"; do
	case $i in
		# various hosts
		ccc_host|carnac_host|ccc_buildbot|carnac_buildbot)
			vmbetter $ROOT/misc/vmbetter_configs/$i.conf
			shift
			;;

		# various hosts (ubuntu version)
		ccc_host_ubuntu|carnac_host_ubuntu|ccc_buildbot_ubuntu|carnac_buildbot_ubuntu)
			vmbetter_ubuntu -O $i $ROOT/misc/vmbetter_configs/${i%_ubuntu}.conf
			shift
			;;


		# various guests (kvm)
		miniccc|minirouter|bro|miniception)
			vmbetter $ROOT/misc/vmbetter_configs/$i.conf
			shift
			;;

		# various guests (container)
		minicccfs|minirouterfs)
			vmbetter -O $i -rootfs $ROOT/misc/vmbetter_configs/${i%fs}_container.conf
			tar -cf - $i | gzip > $i.tar.gz
			shift
			;;

		*)
			echo "unknown vmbetter config"
			break 2
			;;
	esac
done

echo -e "\a"
