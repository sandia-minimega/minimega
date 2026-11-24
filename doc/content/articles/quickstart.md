# Quickstart

This page takes you from an installed minimega to two running Linux VMs on a
private network. You get a console in your browser and SSH from the host. The
page assumes that you finished [Installing minimega](installing.md) by one of
three routes: the `.deb` or `.rpm` package, the Docker image, or a source
build. Where the routes differ, the page shows all three. Everything that you
type at the `minimega$` prompt is the same.

## 1. Start minimega and open a prompt

minimega is a daemon with a command prompt attached. Start it, then attach.

For the package, the service runs the daemon. `-attach` gives you its prompt.

```bash
sudo systemctl start minimega
sudo minimega -attach
```

For Docker, start the container. Then attach inside it.

```bash
docker run -d --name minimega --hostname minimega --privileged --cap-add ALL \
  -p 9000:9000/udp -p 9001:9001 \
  -v /dev:/dev -v /lib/modules:/lib/modules:ro -v /tmp/minimega:/tmp/minimega \
  ghcr.io/sandia-minimega/minimega:latest
docker exec -it minimega minimega -attach
```

[Running in Docker](docker.md) explains the options. The shared
`/tmp/minimega` matters below.

For a source build, run the binary from the repository root. The binary is the
daemon and the prompt at once.

```bash
sudo ./bin/minimega
```

An attached prompt shows the socket path, `minimega:/tmp/minimega/minimega$`.
An interactive prompt shows the namespace, `minimega[minimega]$`. This page
shortens both to `minimega$`. First, make sure that the programs that minimega
depends on are installed and that Open vSwitch is running. No output means
that minimega found no problems:

```minimega
minimega$ check
```

## 2. Build a guest image with vmbetter

vmbetter builds a Debian kernel and initramfs pair from a configuration
file. The `miniccc.conf` configuration produces a small system with SSH,
passwordless root login and the miniccc agent, which is all this page needs.
Build it in the files directory. Then the result is where minimega looks for
files. The directory belongs to root, so the directory change also runs as
root:

```bash
sudo apt install debootstrap         # or: sudo dnf install debootstrap
sudo sh -c 'cd /tmp/minimega/files && vmbetter -level info -branch bookworm /opt/minimega/misc/vmbetter_configs/miniccc.conf'
```

`-branch bookworm` selects the Debian release. Do not use `stable`. It is now
Debian 13 (trixie), which does not have the `ntp` and `dnsutils` packages
that the configuration installs.

In a source build, the configuration is `misc/vmbetter_configs/miniccc.conf`
in the repository, and the binary is `bin/vmbetter`. The Docker image contains
neither `debootstrap` nor the configurations. Run vmbetter on the host
instead:

1. Take the binary from the container.
2. Fetch the configurations from the repository.
3. Build in `/tmp/minimega/files`, which the container shares.

The `miniccc` in the overlay is a symlink to `bin/miniccc` in the repository.
A fresh clone does not have that binary. Copy the binary out of the container
too. If you do not, the build stops with a `cp` error.

```bash
sudo docker cp minimega:/opt/minimega/bin/vmbetter /usr/local/bin/vmbetter
git clone --depth 1 https://github.com/sandia-minimega/minimega.git ~/minimega
mkdir -p ~/minimega/bin
sudo docker cp minimega:/opt/minimega/bin/miniccc ~/minimega/bin/miniccc
sudo sh -c "cd /tmp/minimega/files && vmbetter -level info -branch bookworm $HOME/minimega/misc/vmbetter_configs/miniccc.conf"
```

The build runs `debootstrap` against a Debian mirror. It installs the packages
that the configuration lists and packs the result. The build takes several
minutes and a few hundred megabytes of download. It leaves `miniccc.kernel` and
`miniccc.initrd` in the current directory.
[Building images with vmbetter](vmbetter.md) covers the configuration format
and disk and ISO images.

## 3. Describe the VM

`vm config` holds the description that the next `vm launch` will use. With
nothing set, it shows the defaults:

```minimega
minimega$ vm config
VM configuration:
Memory:           2048
VCPUs:            1
Networks:         []
Bonds:            []
Snapshot:         true
UUID:
Schedule host:
Coschedule limit: -1
Colocate:
Backchannel:      true
Tags:             {}

KVM configuration:
State Path:
Disks:                     []
CDROM Path:
Kernel Path:
Initrd Path:
Kernel Append:             []
QEMU Path:                 kvm
QEMU Append:               []
Serial Ports:              0
Virtio-Serial Ports:
Machine:
Bare metal:                false
Bare metal network driver:
CPU:                       host
Cores:                     0
Threads:                   0
Sockets:                   0
VGA:                       std
Usb Use XHCI:              true
Bidirectional Copy Paste:  false
TPM Socket:

Container configuration:
Filesystem Path:
Hostname:
Init:            [/init]
Pre-init:
FIFOs:           0
Volumes:

Android configuration:
SDK Path:
Emulator Path:
ADB Path:
AVD Name:
AVD Dir:
No Window:         true
Console Base Port: 0
Extra Args:        []
Writable System:   false
GRPC Base Port:    0
```

Two of these matter now. `Snapshot: true` means that disks are copy-on-write
for each VM, so many VMs can share one image without changing it.
`Backchannel: true` gives each VM a virtio-serial channel. miniccc uses that
channel to talk to minimega.

The image unpacks into a RAM-backed file system. This file system can use only
half of the guest memory. A guest with the 2 GB default therefore does not
boot. A relative file
name resolves inside the files directory, so `miniccc.kernel` alone also works
here. These commands use the absolute form, so the same commands work from any
directory and any host. They point the description at the kernel and
initramfs. They also give the VM 3 GB of memory and put the VM on a network
called `LAN`:

```minimega
minimega$ vm config kernel /tmp/minimega/files/miniccc.kernel
minimega$ vm config initrd /tmp/minimega/files/miniccc.initrd
minimega$ vm config memory 3072
minimega$ vm config net LAN
minimega$ vm config
VM configuration:
Memory:           3072
VCPUs:            1
Networks:         [LAN]
...
```

`vm config net` is short for `vm config networks`. `LAN` is a name, not a VLAN
number. minimega allocates a VLAN for it. The VLAN is 101, the first in the
default range. minimega uses that number for every VM and tap that names
`LAN`. `vm info` and `tap` show both below.

## 4. Give the network an address

VMs on `LAN` can only reach each other. A host tap on the same VLAN connects
the host, and a dnsmasq instance on that tap hands out addresses:

```minimega
minimega$ tap create LAN ip 10.0.0.1/24
mega_tap0
minimega$ dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
minimega$ .annotate false tap
bridge      | tap       | vlan
mega_bridge | mega_tap0 | LAN (101)
```

`mega_bridge` is the Open vSwitch bridge that minimega created for you. If
`dnsmasq start` fails, a system dnsmasq is probably holding the port. See
[Installing minimega](installing.md).

## 5. Launch and start two VMs

`vm launch` creates VMs from the description. A number instead of a name
creates that many VMs with generated names. New VMs stay in the `BUILDING`
state until you start them:

```minimega
minimega$ vm launch kvm 2
minimega$ .annotate false .columns name,state,vlan vm info
name | state    | vlan
vm-0 | BUILDING | [LAN (101)]
vm-1 | BUILDING | [LAN (101)]
minimega$ vm start all
```

`vm info` reports everything that minimega knows about each VM. `.columns`
keeps the output readable, and `.annotate false` drops the host column. After
the guests boot and take a DHCP lease, their addresses appear:

```minimega
minimega$ .annotate false .columns name,state,ip vm info
name | state   | ip
vm-0 | RUNNING | [10.0.0.2]
vm-1 | RUNNING | [10.0.0.3]
```

## 6. Connect to a VM

To connect from a browser, use miniweb. It serves a web interface on port
9001. The Docker image already runs miniweb. For the other routes, start it in
a second terminal on the same host:

```bash
sudo miniweb -root /opt/minimega/web     # packages
sudo ./bin/miniweb -root web             # source build, from the repository root
```

Open `http://localhost:9001/`. The VMs page lists both VMs with their state
and address. The Connect link in the VNC column opens a console in a new tab.
The VM Screenshots page shows a live tile for each VM. Click one to connect.
There is no login prompt. The image's `init` starts a root shell on `tty1`, and
the console shows that shell. [miniweb](miniweb.md) describes the rest of the
interface.

To connect with SSH, use the host tap. It gives the host a route to the VMs.
SSH asks for a password, but the password of root is empty:

```bash
ssh root@10.0.0.2      # press Enter at the password prompt
```

## 7. Keep what you did

Two commands turn this session into something that you can repeat. `vm config
save` stores the current description under a name, so later launches can refer
to it (`vm launch kvm 2 quickstart`) without retyping it. `write` saves the
command history as a script:

```minimega
minimega$ vm config save quickstart
minimega$ write /tmp/minimega/files/quickstart.mm
```

The script below is that history after we cleaned it up. Next time, `read`
runs it in one step:

```minimega title="quickstart.mm"
--8<-- "articles/quickstart/quickstart.mm"
```

[Download this example](quickstart/quickstart.mm){ download="quickstart.mm" }

```minimega
minimega$ read /tmp/minimega/files/quickstart.mm
```

[Command line and scripting](cli.md) covers scripts, `minimega -e`, and the
output builtins used above.

## 8. Clean up

Remove the VMs, the DHCP server and the tap, in that order:

```minimega
minimega$ vm kill all
minimega$ vm flush
minimega$ dnsmasq kill all
minimega$ tap delete all
```

`vm kill` leaves VMs in the `QUIT` state, so you can see what happened.
`vm flush` forgets them. When you stop the daemon, it does all of this at
once. Use one of these methods:

- `quit` on a source build.
- `disconnect` and then `sudo systemctl stop minimega` for the service. Under
  systemd, `quit` restarts the daemon.
- `docker stop minimega` for the container.

[Running minimega](running.md) explains the details.

## Next steps

- The [miniclass](../training/miniclass/index.md) course continues from
  here: routers, command and control, captures, clusters.
- [VM lifecycle](vm-lifecycle.md) for every state a VM passes through and the
  commands that move it.
- [Host networking](networking.md) for VLANs, taps, dnsmasq and NAT.
- [Building images with vmbetter](vmbetter.md) for disk images and custom
  guests.

## See also

- [Installing minimega](installing.md)
- [Running minimega](running.md)
- [Command line and scripting](cli.md)
- [Reference: `vm config`](../reference/minimega.md#vm-config)
