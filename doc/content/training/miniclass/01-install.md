# Chapter 1: Install and run minimega

In this chapter you put minimega on a Linux host and start the daemon. You open
a prompt on the daemon and make sure that everything it depends on is in place.
At the end, minimega runs and answers `check` without complaint. You can reach
it from any shell on the host with `minimega -attach` or `minimega -e`. The
course assumes nothing beyond the [prerequisites](index.md#prerequisites) on the
course index.

This chapter has little installation detail on purpose. The
[installation guide](../../articles/installing.md) covers every package, every
runtime dependency, and every host preparation step.
[Running minimega](../../articles/running.md) covers every flag. Here you take
one of three routes (package, Docker, or source) and move on.

## Check the host first

minimega runs KVM virtual machines, so the host needs hardware virtualization
and a kernel with the `kvm` modules loaded. Check both before you install
anything:

```bash
lsmod | grep kvm                              # kvm plus kvm_intel or kvm_amd
cat /sys/module/kvm_intel/parameters/nested   # Y or 1 if the host is itself a VM (Intel)
cat /sys/module/kvm_amd/parameters/nested     # the same on AMD
```

If the host is a virtual machine, `nested` must be on, or every guest you
launch later sits in the `ERROR` state. The
[installation guide](../../articles/installing.md#what-you-need) lists the rest
of the requirements: a 64-bit kernel of 3.18 or later, root, and enough
memory for 2 GB per VM by default (the course gives each VM 3 GB). The
course needs three VMs at once, so a host with 12 GB is comfortable.

## Install

Pick one route. The rest of the course works with any route. Where a path
differs, the course shows all three.

### Package

Every release on
[GitHub Releases](https://github.com/sandia-minimega/minimega/releases) has
a `.deb` and an `.rpm`. Both install the whole tree under `/opt/minimega`,
put `minimega`, `miniweb` and `protonuke` on `PATH`, and create a `minimega`
system user and group.

Debian and Ubuntu:

```bash
VERSION=3.4.0
sudo apt install "./minimega_${VERSION}_amd64.deb"
sudo systemctl enable --now minimega
```

The `.deb` installs QEMU, Open vSwitch, and dnsmasq as dependencies. If `check`
reports `dhclient`, `ssh`, or `scp`, run
`sudo apt install isc-dhcp-client openssh-client`. The package installs its
systemd unit.

RHEL, Rocky, Alma and Fedora:

```bash
VERSION=3.4.0
sudo dnf install epel-release
sudo dnf install "./minimega-${VERSION}-1.x86_64.rpm"
sudo systemctl enable --now openvswitch
sudo systemctl enable minimega
```

The RPM does not depend on Open vSwitch because Open vSwitch is not in the base
repositories. Install the `openvswitch` package that your distribution provides,
and start it before minimega. The RPM installs and starts the service itself.
On these hosts, QEMU is `/usr/libexec/qemu-kvm` and no program named `kvm` is on
`PATH`. Link it (`sudo ln -s /usr/libexec/qemu-kvm /usr/local/bin/kvm`), or set
`vm config qemu /usr/libexec/qemu-kvm` before you launch VMs.

In both cases, the service reads `/etc/minimega/minimega.conf`. This file has
`MM_*` variables that map one to one onto the flags of the daemon. For the list,
see [Running minimega](../../articles/running.md#as-a-service). The defaults are
right for this course.

### Docker

The image bundles minimega, its dependencies, miniweb and this documentation.
It must run privileged, with the host's `/dev` and kernel modules, because QEMU
and Open vSwitch run inside it:

```bash
docker pull ghcr.io/sandia-minimega/minimega:latest
docker run -d --name minimega --hostname minimega --privileged --cap-add ALL \
  -p 9000:9000/udp -p 9001:9001 \
  -v /dev:/dev -v /lib/modules:/lib/modules:ro -v /tmp/minimega:/tmp/minimega \
  ghcr.io/sandia-minimega/minimega:latest
```

The shared `/tmp/minimega` is the base directory of minimega. Because the
container shares it with the host, you can put images into
`/tmp/minimega/files` from the host in the next chapter.
[Running in Docker](../../articles/docker.md) explains every option and the
environment variables of the container.

### Source

You need Go 1.24 or later, a C compiler and the libpcap headers:

```bash
sudo apt install build-essential libpcap-dev
git clone https://github.com/sandia-minimega/minimega.git
cd minimega
./scripts/build.bash
```

`scripts/build.bash` puts every tool in `bin/`. The script does not install the
runtime programs (QEMU, Open vSwitch, dnsmasq, iproute2). The
[runtime dependencies table](../../articles/installing.md#runtime-dependencies)
names the packages. If you build from source, commands in this course that refer
to `/opt/minimega` mean the repository root.

## Start the daemon and open a prompt

minimega is one process. It is the server. If it has a terminal, it is also the
command prompt. The packaged service and the container run it without a
terminal (`-nostdin`), so you attach a prompt to the running daemon. You usually
run a source build in the foreground, where it is the prompt.

Package:

```bash
sudo systemctl status minimega    # should say active (running)
sudo minimega -attach
```

Docker:

```bash
docker exec -it minimega minimega -attach
```

Source:

```bash
sudo ./bin/minimega
```

An attached prompt shows the socket it is connected to,
`minimega:/tmp/minimega/minimega$`. An interactive prompt shows the active
namespace, `minimega[minimega]$`. The course shortens both to `minimega$`.
Leave an attached prompt with `disconnect` or Ctrl-D. The daemon keeps running.
Type `quit` there only to stop the daemon.

The socket is in the base directory, `/tmp/minimega` by default. Everything that
talks to minimega uses the socket: `-attach`, `-e`, miniweb, and the Python
bindings. For this reason the Docker route shares that directory with the host.
For the same reason, a daemon that you start with a different `-base` needs the
same `-base` on every client.

## Check that it works

Three commands tell you that the install is sound. `version` shows the version
that you run:

```minimega
minimega$ version
```

`check` looks for every external program that minimega calls. It makes sure that
the programs have the minimum versions and that Open vSwitch is running. If all
is well, it prints nothing. For each item that is missing, it prints an error:

```minimega
minimega$ check
```

A missing program breaks only the features that use it. But the course needs the
whole set: QEMU for VMs, Open vSwitch for networks, and dnsmasq for the host
DHCP server in chapter 6. Fix each problem that `check` reports before you go
on. The most common report on RHEL-family hosts is the missing `kvm` name that
the text above describes. The most common report on any host is Open vSwitch not
started.

`help` lists every command with a one-line summary. `help <command>` prints the
full text with the exact patterns that the command accepts:

```minimega
minimega$ help
minimega$ help vm launch
```

The prompt completes command words with Tab and keeps history, so
`help` plus Tab is a fast way to explore.

## Commands from the shell

You can send from a shell each command that you type at the prompt. `-e` runs
one command on the daemon, prints the answer, and exits. Scripts and the rest of
this course use it for quick checks:

```bash
sudo minimega -e version
sudo minimega -e check
sudo minimega -e .columns name,state vm info
```

From the Docker route, prefix these with `docker exec minimega`, or use the
container's `mm` shortcut: `docker exec minimega mm vm info`.

Under the packaged service, members of the `minimega` group can attach and
run `-e` without `sudo`. Add yourself to the group. Then log in again:

```bash
sudo usermod -aG minimega "$USER"
```

Being in that group is equivalent to root on the host, because minimega runs as
root and runs anything that you ask. See
[Security considerations](../../articles/security.md).

The binary generates tab completion for the `minimega` command itself, including
the minimega commands after `-e`. The packages and the Docker image already
install it for bash. To use it in the current shell only, run:

```bash
source <(minimega -completion bash)
```

`-completion zsh` and `-completion fish` produce the equivalents for those
shells. For where to install them, see
[Running minimega](../../articles/running.md#shell-completion).

## Two host services that get in the way

When the first network exists, both services cause problems. Fix them now.

Installing the `dnsmasq` package on Debian and Ubuntu enables a system-wide
dnsmasq that listens on every interface. minimega starts its own instances on
demand, and they fail to bind while the system one holds the port. minimega
only needs the binary:

```bash
sudo systemctl disable --now dnsmasq
```

On desktop hosts, NetworkManager can take over the `mega_bridge` and
`mega_tap*` interfaces that minimega creates. Tell it not to, as described under
[services that fight over the bridge](../../articles/installing.md#services-that-fight-over-the-bridge).

## Stop, restart, and clean up

`quit` at the prompt stops the daemon. This destroys every VM, tap, and bridge
that the daemon created. On an attached prompt, you type it twice. Under
systemd, the unit restarts the daemon after a clean exit
(`Restart=on-success`). To keep the service down, use `systemctl`:

```bash
sudo systemctl stop minimega       # package
docker stop minimega               # Docker
```

A source build in the foreground stops with `quit` or Ctrl-D.

If a daemon crashes or is killed, it can leave QEMU processes, taps, and the
`mega_bridge` behind. Then the next start refuses to run over the stale socket:
`minimega appears to already be running, override with -force`. Start the
daemon once with `-force`. Then run `nuke`:

```bash
sudo minimega -force -nostdin &
sudo minimega -e nuke
```

Under the packaged service `MM_FORCE` is already `true` in
`minimega.conf`, so `sudo systemctl start minimega` followed by
`sudo minimega -e nuke` does the same.

`nuke` kills the QEMU and dnsmasq processes that minimega recorded under the
base directory. It deletes every `mega_tap*` interface and removes the bridges
that minimega created. It also deletes the base directory and exits. It is the
most drastic reset. On a healthy daemon, `clear all` resets the state without
stopping the process. If the VMs of the old daemon still run and you want to
keep them, `-recover` adopts them instead. See
[Recovering from a previous instance](../../articles/running.md#recovering-from-a-previous-instance).

## What you built

- minimega installed from a package, the Docker image, or source, with its
  runtime programs in place and Open vSwitch running.
- A daemon you can reach from any shell with `minimega -attach` and
  `minimega -e`.
- A host whose system dnsmasq and NetworkManager do not conflict with minimega
  over its bridge.

## Where to read more

- [Installing minimega](../../articles/installing.md): every package,
  dependency and host preparation step.
- [Running minimega](../../articles/running.md): the service, `minimega.conf`,
  every flag, logging, recovery.
- [Running in Docker](../../articles/docker.md): the container in detail.
- [Command line and scripting](../../articles/cli.md): the prompt, output
  builtins, command files.
- Reference: [`check`](../../reference/minimega.md#check),
  [`help`](../../reference/minimega.md#help),
  [`version`](../../reference/minimega.md#version),
  [`quit`](../../reference/minimega.md#quit),
  [`nuke`](../../reference/minimega.md#nuke).

## Next

[Chapter 2: Get VM images](02-images.md) builds the two guest images that
the course runs on.
