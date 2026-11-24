# Security considerations

minimega is built to run experiments on hosts that you control, on a management
network that you trust. It has almost no internal access control. Whoever can
reach one of its interfaces can do everything that minimega can do, and
minimega runs as root. This page states what each interface exposes and what
the code does about it. Use this information to decide where to put the
security boundary. Before you put a minimega host on a shared network or give
other people access to it, read this page.

## minimega runs as root

These tasks all need root: creating taps and bridges, driving KVM, mounting
guest filesystems and managing cgroups. The systemd unit in `misc/daemon/`
starts the daemon as root.

Anything that can issue commands to minimega
therefore has root on the host. The `shell` and `background` commands run
arbitrary host programs. `cc mount` mounts filesystems. `disk` writes images.
`read` executes command files.

## The command socket

Every client of the daemon talks to the UNIX socket `<base>/minimega` (default
`/tmp/minimega/minimega`) with an unauthenticated JSON protocol. The clients
include `minimega -e`, `minimega -attach`, miniweb, the Python bindings and
phēnix. Filesystem permissions are the only control on the socket.

minimega creates the base directory with mode `0770`. The `.deb` and `.rpm`
packages create a system user and group named `minimega`. After startup, the
systemd unit runs `chgrp -R minimega` and `chmod -R g=u` over the base
directory. It also sets the setgid bit on the base directory and on the socket.
Members of the `minimega` group can therefore use the socket without `sudo`.

Membership in that group gives root on the host. The same applies to the base
directory as a whole. It holds the QMP and serial sockets of each VM, which give
control of the VM, and the response files that miniccc writes.

## The mesh

Cluster nodes find each other by UDP broadcast and connect over TCP port
9000 (`-port`). The handshake exchanges a node name, a solicitation flag and
the version string. It has no authentication and no encryption. A host that can
reach the port and uses the same `-context` joins the mesh. Any member can then
run commands on any other member with `mesh send`.

Keep the mesh on an isolated management network. `deploy` copies the binary and
starts remote nodes over SSH with the credentials of the user who runs it.

## VM consoles

For each KVM VM, minimega opens a TCP listener on all of the addresses of the
host. The listener uses a random port that the `vnc_port` column of `vm info`
shows. minimega proxies the listener to the VNC socket of QEMU. minimega starts
QEMU with VNC on a UNIX socket and no VNC password. Anyone who can reach the
addresses of the host can open the console of any VM, watch it and type into
it. A host firewall that admits only the management network to those ports
mitigates this.

## miniweb

miniweb serves HTTP on `:9001` by default, on all addresses. Without
`-passwords`, it requires no login. A visitor can then list VMs and hosts,
start, stop and kill VMs, and launch new VMs. A visitor can also download any
file under the `-filepath` directory of minimega and connect to VM consoles.
With `-console <path to minimega>`, miniweb also exposes:

- `/console`, a browser terminal that runs `minimega -attach`. It gives the full
  CLI, with `shell`.
- `/command`, which runs a POSTed minimega command and returns the result.
- `/commands`, which runs a POSTed JSON list of minimega commands.

Without `-console` those three paths return `501 Not Implemented`.

Authentication is HTTP basic auth against a JSON password file. The file has
bcrypt-hashed entries, and each entry is scoped to a URL path prefix.
`-bootstrap` writes the file interactively. It requires `-passwords` to say
where.

A rule applies to every path that starts with its string. A rule for `/` covers
the whole site. A rule for `/vm/webserver/` limits a user to one VM. Without the
final `/`, the rule also matches `/vm/webserver2`. A path that matches no rule
needs no password, so also keep a rule for `/`. Basic auth sends the password
with every request, so enable TLS with `-cert` and `-key`. Both flags are
required. If you give only one, miniweb reports a startup error.

`-namespace <name>` pins every command miniweb issues to that namespace and
disables the namespaces pages. Without that flag, the `namespace` query
parameter on any request selects the namespace. A per-path password rule for one
namespace therefore does not by itself stop a user from naming another.

The Docker image's start script launches miniweb on `0.0.0.0:9001` with no
password file. See [miniweb](miniweb.md) and [Running in Docker](docker.md).

## miniccc and the guest

The agent executes whatever minimega sends it, as the user that the agent runs
as. In the stock Linux images, that user is root. When you install the agent as
a Windows service, that user is `LocalSystem`. The host can run any program,
read and write any file, open tunnels in both directions and change the log
level of the agent. `cc mount` serves `/`, or the system drive on Windows,
read/write. The channel has no authentication or encryption in either
direction.

Which side can reach the channel is what matters:

- minimega creates the virtio-serial port and the container UNIX socket, and
  they exist only on the host. Nothing on the experiment network can reach
  them.
- `cc listen <port>` opens a TCP listener on all of the addresses of the host.
  It accepts any connection that presents the UUID of a VM in the namespace.
  `vm info` and every guest show UUIDs. If something other than a guest that
  you trust can reach the listener, do not use TCP mode. We recommend a tap on
  an isolated VLAN instead of a management interface.

Data flows upward too. Hostname, addresses, tags and command output come from
the guest, and minimega stores and displays them unchanged. `cc filter` matches
on them, so a guest that changes its hostname can change which commands it
receives.

`cc tunnel` opens a listener on the minimega host. The listener reaches whatever
the tunneling guest can reach. A working reverse tunnel can give guests a path
to whatever the host can reach.

Treat the host as part of the experiment boundary. Keep the host isolated.

## Files

The `file` API serves the `-filepath` directory (`/tmp/minimega/files` by
default) to every node in the mesh. `cc send` serves it to guests, and miniweb
serves it to visitors under `/files/`. Keep only files in that directory that
the least trusted of those readers can read. Responses and files received from
guests are stored under `miniccc_responses` in the same tree.

## Passwords in images

Images that vmbetter builds carry whatever credentials their configurations and
overlays set. `passwordify` rewrites an initramfs. It runs `passwd` in a chroot
to set the password of root. It installs an `authorized_keys` file for root from
`-keys`.

By default (`-passwordless=true`), `passwordify` also generates an SSH key pair
inside the image. It adds the public half of the key pair to `authorized_keys`.
Every VM that boots from that initrd then trusts the same private key and can
log into every other VM as root. If you do not want this, pass
`-passwordless=false`.

```bash
$ passwordify -keys ~/.ssh/id_ed25519.pub -passwordless=false in.initrd out.initrd
```

## See also

- [Running minimega](running.md) for the systemd unit and base directory.
- [miniweb](miniweb.md) for the password file format.
- [Command and control](cc.md) for the transports.
- [Cluster setup](cluster.md) for the mesh.
- [Building images with vmbetter](vmbetter.md).
