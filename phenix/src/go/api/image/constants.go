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

██████╗ ██╗  ██╗███████╗███╗  ██╗██╗██╗  ██╗
██╔══██╗██║  ██║██╔════╝████╗ ██║██║╚██╗██╔╝
██████╔╝███████║█████╗  ██╔██╗██║██║ ╚███╔╝
██╔═══╝ ██╔══██║██╔══╝  ██║╚████║██║ ██╔██╗
██║     ██║  ██║███████╗██║ ╚███║██║██╔╝╚██╗
╚═╝     ╚═╝  ╚═╝╚══════╝╚═╝  ╚══╝╚═╝╚═╝  ╚═╝

EOF
echo "\nBuilt with phenix image on $(date)\n\n" >> /etc/motd
`

const POSTBUILD_PHENIX_BASE = `
mkdir -p /etc/phenix/startup
mkdir -p /etc/systemd/system/multi-user.target.wants
mkdir -p /usr/local/bin
cat > /etc/systemd/system/miniccc.service <<EOF
[Unit]
Description=miniccc
[Service]
ExecStart=/usr/local/bin/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /var/log/miniccc.log
[Install]
WantedBy=multi-user.target
EOF
cat > /etc/systemd/system/phenix.service <<EOF
[Unit]
Description=phenix startup service
After=network.target systemd-hostnamed.service
[Service]
Environment=LD_LIBRARY_PATH=/usr/local/lib
ExecStart=/usr/local/bin/phenix-start.sh
RemainAfterExit=true
StandardOutput=journal
Type=oneshot
[Install]
WantedBy=multi-user.target
EOF
ln -s /etc/systemd/system/miniccc.service /etc/systemd/system/multi-user.target.wants/miniccc.service
ln -s /etc/systemd/system/phenix.service /etc/systemd/system/multi-user.target.wants/phenix.service
cat > /usr/local/bin/phenix-start.sh <<EOF
#!/bin/bash
for file in /etc/phenix/startup/*; do
	echo $file
	bash $file
done
EOF
chmod +x /usr/local/bin/phenix-start.sh
`

const POSTBUILD_PROTONUKE = `
mkdir -p /etc/systemd/system/multi-user.target.wants
mkdir -p /usr/local/bin
cat > /usr/local/bin/wup <<EOF
#!/bin/bash
# [w]ait [u]ntil [p]resent
[ -n "$1" ] && \
	until ip link show ${1} &> /dev/null; do sleep 1; done
exit $?
EOF
chmod +x /usr/local/bin/wup
cat > /etc/systemd/system/protonuke.service <<EOF
[Unit]
Description=protonuke
[Service]
EnvironmentFile=/etc/default/protonuke
ExecStartPre=/usr/local/bin/wup ens1
ExecStart=/usr/local/bin/protonuke $PROTONUKE_ARGS
[Install]
WantedBy=multi-user.target
EOF
ln -s /etc/systemd/system/protonuke.service /etc/systemd/system/multi-user.target.wants/protonuke.service
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
	"xfce4",
	"python3-cffi-backend",
}

var PACKAGES_MINGUI_KALI = []string{
	"ca-certificates-java",
	"openjdk-8-jre-headless",
	"kali-desktop-xfce",
}

var PACKAGES_BRASH = []string{
	"vsftpd",
	"socat",
	"telnet",
	"ftp",
	"pv",
	"python3",
}

var PACKAGES_COMPONENTS = []string{
	"main",
	"restricted",
	"universe",
	"multiverse",
}

var PACKAGES_COMPONENTS_KALI = []string{
	"main",
	"contrib",
	"non-free",
}
