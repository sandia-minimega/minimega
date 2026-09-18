# Security considerations

minimega is built to run experiments on hosts you control, on a management
network you trust. It has almost no internal access control: whoever can
reach one of its interfaces can do everything it can, and it runs as root.
This page states what each interface exposes and what the code does about it,
so you can decide where to draw the boundary. Read it before you put a
minimega host on a shared network or give other people access to it.

## minimega runs as root

Creating taps and bridges, driving KVM, mounting guest filesystems and
managing cgroups all need root, and the systemd unit in `misc/daemon/`
starts the daemon as root. Anything that can issue commands to minimega
therefore has root on the host: the `shell` and `background` commands run
arbitrary host programs, `cc mount` mounts filesystems, `disk` writes images,
and `read` executes command files.

## The command socket

Every client of the daemon, including `minimega -e`, `minimega -attach`,
miniweb, the Python bindings and phēnix, talks to the UNIX socket
`<base>/minimega` (default `/tmp/minimega/minimega`) with an unauthenticated
JSON protocol. Filesystem permissions are the only control on it.

minimega creates the base directory with mode `0770`. The `.deb` and `.rpm`
packages create a system user and group named `minimega`, and after startup the systemd
unit runs `chgrp -R minimega` and `chmod -R g=u` over the base directory and
sets the setgid bit on it and on the socket, so members of the `minimega`
group can use the socket without `sudo`. Membership in that group is
equivalent to root on the host. The same applies to the base directory as a
whole: it holds each VM's QMP and serial sockets, which give control of the
VM, and the response files written by miniccc.

## The mesh

Cluster nodes find each other by UDP broadcast and connect over TCP port
9000 (`-port`). The handshake exchanges a node name, a solicitation flag and
the version string; there is no authentication and no encryption. A host that
can reach the port and uses the same `-context` joins the mesh, and any
member can run commands on any other with `mesh send`. Keep the mesh on an
isolated management network. `deploy` copies the binary and starts remote
nodes over SSH with the credentials of the user running it.

## VM consoles

For each KVM VM, minimega opens a TCP listener on all of the host's
addresses, at a random port shown in the `vnc_port` column of `vm info`, and
proxies it to QEMU's VNC socket. QEMU is started with VNC on a UNIX socket
and no VNC password. Anyone who can reach the host's addresses can open any
VM's console, watch it and type into it. A host firewall that admits only the
management network to those ports is the mitigation.

## miniweb

miniweb serves HTTP on `:9001` by default, on all addresses, and without
`-passwords` it requires no login. Through it a visitor can list VMs and
hosts, start, stop and kill VMs, launch new ones, download any file under
minimega's `-filepath` directory, and connect to VM consoles. With
`-console <path to minimega>` it additionally exposes:

- `/console`, a browser terminal running `minimega -attach`, that is, the
  full CLI including `shell`;
- `/command`, which runs a POSTed minimega command and returns the result;
- `/commands`, the command list.

Without `-console` those three paths return `501 Not Implemented`.

Authentication is HTTP basic auth against a JSON password file of
bcrypt-hashed entries, each scoped to a URL path prefix; `-bootstrap` writes
the file interactively and requires `-passwords` to say where. Rules are
inherited by longer paths, so a rule for `/` covers the whole site and a rule
for `/vm/webserver` limits a user to one VM. Basic auth sends the password with
every request, so enable TLS with `-cert` and `-key`; both are required, and
giving only one is a startup error.

`-namespace <name>` pins every command miniweb issues to that namespace and
disables the namespaces pages. Without it, the `namespace` query parameter on
any request selects the namespace, so a per-path password rule for one
namespace does not by itself stop a user from naming another.

The Docker image's start script launches miniweb on `0.0.0.0:9001` with no
password file. See [miniweb](miniweb.md) and [Running in Docker](docker.md).

## miniccc and the guest

The agent executes whatever minimega sends it, as the user it runs as: root in
the stock Linux images, and `LocalSystem` when installed as a Windows
service. The host can run any program, read and write any file (`cc mount`
serves `/`, or the system drive on Windows, read/write), open tunnels in both
directions and change the agent's log level. There is no authentication or
encryption on the channel in either direction.

Which side can reach the channel is what matters:

- The virtio-serial port and the container UNIX socket are created by
  minimega and exist only on the host. Nothing on the experiment network can
  reach them.
- `cc listen <port>` opens a TCP listener on all of the host's addresses.
  It accepts any connection that presents the UUID of a VM in the namespace,
  and UUIDs are visible in `vm info` and inside every guest. Only use TCP
  mode where the listener is reachable solely by guests you trust, and
  prefer a tap on an isolated VLAN over a management interface.

Data flows upward too. Hostname, addresses, tags and command output come from
the guest and are stored and displayed unchanged; `cc filter` matches on
them, so a guest that changes its hostname can change which commands it
receives. `cc tunnel` opens a listener on the minimega host that reaches
whatever the tunnelling guest can reach, and a working reverse tunnel would
give guests a path to whatever the host can reach; treat the host as part of
the experiment boundary and keep it isolated.

## Files

The `-filepath` directory (`/tmp/minimega/files` by default) is served to
every node in the mesh by the `file` API, to guests by `cc send`, and to
miniweb visitors under `/files/`. Do not keep anything there that the least
trusted of those should not read. Responses and files received from guests
land under `miniccc_responses` in the same tree.

## Passwords in images

Images built with vmbetter carry whatever credentials their configs and
overlays set. `passwordify` rewrites an initramfs: it runs `passwd` in a
chroot to set root's password, installs an `authorized_keys` file for root
from `-keys`, and by default (`-passwordless=true`) generates an SSH key pair
inside the image and adds its public half to `authorized_keys`, so every VM
booted from that initrd trusts the same private key and can log into every
other one as root. Pass `-passwordless=false` unless that is what you want.

```bash
$ passwordify -keys ~/.ssh/id_ed25519.pub -passwordless=false in.initrd out.initrd
```

## See also

- [Running minimega](running.md) for the systemd unit and base directory.
- [miniweb](miniweb.md) for the password file format.
- [Command and control](cc.md) for the transports.
- [Cluster setup](cluster.md) for the mesh.
- [Building images with vmbetter](vmbetter.md).
