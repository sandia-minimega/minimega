# Quickstart

This page takes you from an installed minimega to two running Linux VMs on a
private network, with a console in your browser and SSH from the host. It
assumes you have finished [Installing minimega](installing.md) by one of three
routes: the `.deb` or `.rpm` package, the Docker image, or a source build.
Where the routes differ, all three are shown; everything typed at the
`minimega$` prompt is the same.

## 1. Start minimega and open a prompt

minimega is a daemon with a command prompt attached. Start it, then attach.

**Package.** The service runs the daemon; `-attach` gives you its prompt.

```bash
sudo systemctl start minimega
sudo minimega -attach
```

If `systemctl` cannot find the unit on Debian or Ubuntu, link it in as
described in [Installing minimega](installing.md).

**Docker.** Start the container, then attach inside it.

```bash
docker run -d --name minimega --hostname minimega --privileged --cap-add ALL \
  -p 9000:9000/udp -p 9001:9001 \
  -v /dev:/dev -v /lib/modules:/lib/modules:ro -v /tmp/minimega:/tmp/minimega \
  ghcr.io/sandia-minimega/minimega:latest
docker exec -it minimega minimega -attach
```

[Running in Docker](docker.md) explains the options; the shared
`/tmp/minimega` matters below.

**Source.** Run the binary from the repository root; it is the daemon and the
prompt at once.

```bash
sudo ./bin/minimega
```

Attached prompts show the socket path, `minimega:/tmp/minimega/minimega$`, and
an interactive one shows the namespace, `minimega[minimega]$`. This page
shortens both to `minimega$`. First check that the programs minimega depends
on are installed and Open vSwitch is running; no output means all is well:

```minimega
minimega$ check
```

## 2. Build a guest image with vmbetter

vmbetter builds a Debian kernel and initramfs pair from a configuration
file. The `miniccc.conf` configuration produces a small system with SSH,
passwordless root login and the miniccc agent, which is all this page needs.
Build it from the files directory so the result lands where minimega looks
for files:

```bash
sudo apt install debootstrap         # or: sudo dnf install debootstrap
cd /tmp/minimega/files
sudo vmbetter -level info -branch stable /opt/minimega/misc/vmbetter_configs/miniccc.conf
```

From a source build, the configuration is `misc/vmbetter_configs/miniccc.conf`
in the repository and the binary is `bin/vmbetter`. The Docker image contains
neither `debootstrap` nor the configurations, so run vmbetter on the host
instead: take the binary from the container, fetch the configurations from
the repository, and build in `/tmp/minimega/files`, which the container
shares. The overlay's `miniccc` is a symlink to `bin/miniccc` in the
repository, which a fresh clone does not have, so copy that binary out of
the container too or the build stops with a `cp` error.

```bash
sudo docker cp minimega:/opt/minimega/bin/vmbetter /usr/local/bin/vmbetter
git clone --depth 1 https://github.com/sandia-minimega/minimega.git ~/minimega
mkdir -p ~/minimega/bin
sudo docker cp minimega:/opt/minimega/bin/miniccc ~/minimega/bin/miniccc
cd /tmp/minimega/files
sudo vmbetter -level info -branch stable ~/minimega/misc/vmbetter_configs/miniccc.conf
```

The build runs `debootstrap` against a Debian mirror, installs the packages
the configuration lists, and packs the result; it takes several minutes and
a few hundred megabytes of download. It leaves `miniccc.kernel` and
`miniccc.initrd` in the current directory.
[Building images with vmbetter](vmbetter.md) covers the configuration format
and disk and ISO images.

## 3. Describe the VM

`vm config` holds the description that the next `vm launch` will use. With
nothing set it shows the defaults:

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

Two of these matter now. `Snapshot: true` means disks are copy-on-write per
VM, so many VMs can share one image without changing it. `Backchannel: true`
gives each VM a virtio-serial channel that miniccc uses to talk to minimega.
Point the description at the kernel and initramfs, and put the VM on a
network called `LAN`. A relative file name resolves inside the files
directory, so `miniccc.kernel` alone would also work here; the absolute form
is used so the same commands work from any directory and any host:

```minimega
minimega$ vm config kernel /tmp/minimega/files/miniccc.kernel
minimega$ vm config initrd /tmp/minimega/files/miniccc.initrd
minimega$ vm config networks LAN
minimega$ vm config
VM configuration:
Memory:           2048
VCPUs:            1
Networks:         [LAN]
...
```

`LAN` is a name, not a VLAN number. minimega allocates a VLAN for it (101,
the first in the default range) and uses that number for every VM and tap
that names `LAN`; `vm info` and `tap` show both below.

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

`mega_bridge` is the Open vSwitch bridge minimega created for you. If
`dnsmasq start` fails, a system dnsmasq is probably holding the port; see
[Installing minimega](installing.md).

## 5. Launch and start two VMs

`vm launch` creates VMs from the description; a number instead of a name
creates that many with generated names. New VMs sit in the `BUILDING` state
until you start them:

```minimega
minimega$ vm launch kvm 2
minimega$ .annotate false .columns name,state,vlan vm info
name | state    | vlan
vm-0 | BUILDING | [LAN (101)]
vm-1 | BUILDING | [LAN (101)]
minimega$ vm start all
```

`vm info` reports everything minimega knows about each VM; `.columns` keeps
the output readable and `.annotate false` drops the host column. Once the
guests have booted and taken a DHCP lease, their addresses appear:

```minimega
minimega$ .annotate false .columns name,state,ip vm info
name | state   | ip
vm-0 | RUNNING | [10.0.0.2]
vm-1 | RUNNING | [10.0.0.3]
```

## 6. Connect to a VM

**Browser.** miniweb serves a web interface on port 9001. The Docker image
already runs it; otherwise start it in a second terminal on the same host:

```bash
sudo miniweb -root /opt/minimega/web     # packages
sudo ./bin/miniweb -root web             # source build, from the repository root
```

Open `http://localhost:9001/`. The VMs page lists both VMs with their state
and address; the Connect link in the VNC column opens a console in a new tab.
The VM Screenshots page shows a live tile per VM; click one to connect. There
is no login prompt: the image's `init` starts a root shell on `tty1`, and that
is what the console shows. [miniweb](miniweb.md) describes the rest of
the interface.

**SSH.** The host tap gives the host a route to the VMs. SSH does ask for a
password; root's is empty:

```bash
ssh root@10.0.0.2      # press Enter at the password prompt
```

## 7. Keep what you did

Two commands turn this session into something you can repeat. `vm config
save` stores the current description under a name, so later launches can
refer to it (`vm launch kvm 2 quickstart`) without retyping it. `write` saves
the command history as a script:

```minimega
minimega$ vm config save quickstart
minimega$ write /tmp/minimega/files/quickstart.mm
```

The script below is that history, tidied. Next time, `read` runs it in one go:

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

`vm kill` leaves VMs in the `QUIT` state so you can see what happened;
`vm flush` forgets them. Stopping the daemon does all of this at once: `quit`
on a source build, `disconnect` and then `sudo systemctl stop minimega` for
the service (under systemd, `quit` restarts the daemon), or
`docker stop minimega` for the container. [Running minimega](running.md)
explains the details.

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
