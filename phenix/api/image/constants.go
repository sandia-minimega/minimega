package image

const POSTBUILD_APT_CLEANUP = `
apt clean || apt-get clean || echo "unable to clean apt cache"
`

const POSTBUILD_NO_ROOT_PASSWD = `
sed -i 's/nullok_secure/nullok/' /etc/pam.d/common-auth
sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
sed -i 's/#PermitEmptyPasswords no/PermitEmptyPasswords yes/' /etc/ssh/sshd_config
sed -i 's/PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
sed -i 's/PermitEmptyPasswords no/PermitEmptyPasswords yes/' /etc/ssh/sshd_config
passwd -d root
`

const POSTBUILD_PHENIX_HOSTNAME = `
echo "phenix" > /etc/hostname
sed -i 's/127.0.1.1 .*/127.0.1.1 phenix/' /etc/hosts
cat > /etc/motd <<EOF

███████╗ ██████╗███████╗██████╗ ████████╗███████╗███████╗
██╔════╝██╔════╝██╔════╝██╔══██╗╚══██╔══╝██╔══██║██╔════╝
███████╗██║     █████╗  ██████╔╝   ██║   ██████╔╝█████╗
╚════██║██║     ██╔══╝  ██╔═══╝    ██║   ██╔══██╗██╔══╝
███████║╚██████╗███████╗██║        ██║   ██║  ██║███████╗
╚══════╝ ╚═════╝╚══════╝╚═╝        ╚═╝   ╚═╝  ╚═╝╚══════╝

EOF
echo "\nBuilt with phenix image on $(date)\n\n" >> /etc/motd
`

const POSTBUILD_BRASH = `
echo "root:SiaSd3te" | chpasswd
adduser sceptre --UID 1001 --gecos "" --shell /usr/bin/bennu-brash --disabled-login
echo "sceptre:sceptre" | chpasswd
sed -i 's/$DISTRIB_DESCRIPTION/SCEPTRE 5.0 LTS/g' /etc/update-motd.d/00-header
`

const POSTBUILD_ENABLE_DHCP = `
echo "#!/bin/bash\ndhclient" > /etc/init.d/dhcp.sh
chmod +x /etc/init.d/dhcp.sh
update-rc.d dhcp.sh defaults 100
`

var PACKAGES_DEFAULT = []string{
	"initramfs-tools",
	"net-tools",
	"isc-dhcp-client",
	"openssh-server",
	"init",
	"iputils-ping",
	"vim",
	"less",
	"netbase",
	"curl",
	"ifupdown",
	"dbus",
}

var PACKAGES_KALI = []string{
	"linux-image-amd64",
	"linux-headers-amd64",
}

var PACKAGES_BIONIC = []string{
	"linux-image-generic",
	"linux-headers-generic",
}

var PACKAGES_MINGUI = []string{
	"xorg",
	"xserver-xorg-input-all",
	"dbus-x11",
	"xserver-xorg-video-qxl",
	"xserver-xorg-video-vesa",
	"xinit",
	"xfce4-terminal",
	"qupzilla",
}

var PACKAGES_MINGUI_KALI = []string{
	"ca-certificates-java",
	"openjdk-8-jre-headless",
	"kali-desktop-xfce",
}

var PACKAGES_MINGUI_BIONIC = []string{
	"xfce4",
}

var PACKAGES_BRASH = []string{
	"vsftpd",
	"socat",
	"telnet",
	"ftp",
	"pv",
	"python3",
}
