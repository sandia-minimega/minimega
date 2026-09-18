# Chapter 1: Install and run minimega

In this chapter you put minimega on a Linux host, start the daemon, open a
prompt on it, and confirm that everything it depends on is in place. At the
end you have a running minimega that answers `check` without complaint and
that you can reach from any shell on the host with `minimega -attach` or
`minimega -e`. Nothing is assumed beyond the
[prerequisites](index.md#prerequisites) on the course index.

This chapter is deliberately short on installation detail. The
[installation guide](../../articles/installing.md) covers every package, every
runtime dependency and every host preparation step;
[Running minimega](../../articles/running.md) covers every flag. Here you take
one of three routes, package, Docker, or source, and move on.

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
memory for 2 GB per VM by default. The course needs three VMs at once, so a
host with 8 GB is comfortable.

## Install

Pick one route. The rest of the course does not care which, except where a
path differs, and those places show all three.

### Package

Every release on
[GitHub Releases](https://github.com/sandia-minimega/minimega/releases) has
a `.deb` and an `.rpm`. Both install the whole tree under `/opt/minimega`,
put `minimega`, `miniweb` and `protonuke` on `PATH`, and create a `minimega`
system user and group.

Debian and Ubuntu:

```bash
VERSION=3.2.0
sudo apt install "./minimega_${VERSION}_amd64.deb"
sudo ln -s /opt/minimega/misc/daemon/minimega.service /etc/systemd/system/minimega.service
sudo systemctl daemon-reload
sudo systemctl enable --now minimega
```

The `.deb` pulls in QEMU, Open vSwitch, dnsmasq and the other runtime
programs, but ships its systemd unit without installing it, which is what the
`ln -s` is for.

RHEL, Rocky, Alma and Fedora:

```bash
VERSION=3.2.0
sudo dnf install epel-release
sudo dnf install "./minimega-${VERSION}-1.x86_64.rpm"
sudo systemctl enable --now openvswitch
sudo systemctl enable minimega
```

The RPM does not depend on Open vSwitch because it is not in the base
repositories; install the `openvswitch` package your distribution provides
and start it before minimega. The RPM installs and starts the service itself.
On these hosts QEMU is `/usr/libexec/qemu-kvm` and nothing named `kvm` is on
`PATH`, so either link it (`sudo ln -s /usr/libexec/qemu-kvm /usr/local/bin/kvm`)
or set `vm config qemu /usr/libexec/qemu-kvm` before launching VMs.

Either way the service reads `/etc/minimega/minimega.conf`, a file of `MM_*`
variables that map one to one onto the daemon's flags; see
[Running minimega](../../articles/running.md#as-a-service) for the list. The
defaults are right for this course.

### Docker

The image bundles minimega, its dependencies, miniweb and this documentation.
It has to run privileged, with the host's `/dev` and kernel modules, because
QEMU and Open vSwitch run inside it:

```bash
docker pull ghcr.io/sandia-minimega/minimega:latest
docker run -d --name minimega --hostname minimega --privileged --cap-add ALL \
  -p 9000:9000/udp -p 9001:9001 \
  -v /dev:/dev -v /lib/modules:/lib/modules:ro -v /tmp/minimega:/tmp/minimega \
  ghcr.io/sandia-minimega/minimega:latest
```

The shared `/tmp/minimega` is minimega's base directory. Sharing it with the
host is what lets you drop images into `/tmp/minimega/files` from the host
in the next chapter. [Running in Docker](../../articles/docker.md) explains
every option and the container's environment variables.

### Source

You need Go 1.24 or later, a C compiler and the libpcap headers:

```bash
sudo apt install build-essential libpcap-dev
git clone https://github.com/sandia-minimega/minimega.git
cd minimega
./scripts/build.bash
```

`scripts/build.bash` puts every tool in `bin/`. The runtime programs (QEMU,
Open vSwitch, dnsmasq, iproute2) are not installed for you; the
[runtime dependencies table](../../articles/installing.md#runtime-dependencies)
names the packages. Commands in this course that refer to `/opt/minimega`
mean the repository root when you build from source.

## Start the daemon and open a prompt

minimega is one process that is both the server and, when it has a terminal,
the command prompt. The packaged service and the container run it without a
terminal (`-nostdin`), so you attach a prompt to the running daemon; a source
build is usually run in the foreground, where it is the prompt.

**Package**

```bash
sudo systemctl status minimega    # should say active (running)
sudo minimega -attach
```

**Docker**

```bash
docker exec -it minimega minimega -attach
```

**Source**

```bash
sudo ./bin/minimega
```

An attached prompt shows the socket it is connected to,
`minimega:/tmp/minimega/minimega$`, and an interactive one shows the active
namespace, `minimega[minimega]$`. The course shortens both to `minimega$`.
Leave an attached prompt with `disconnect` or Ctrl-D; the daemon keeps
running. Do not type `quit` there unless you mean to stop the daemon.

The socket lives in the base directory, `/tmp/minimega` by default. Anything
that talks to minimega, `-attach`, `-e`, miniweb, the Python bindings, uses
it, which is why the Docker route shares that directory with the host and why
a daemon started with a different `-base` needs the same `-base` on every
client.

## Check that it works

Three commands tell you the install is sound. `version` shows what you are
running:

```minimega
minimega$ version
```

`check` looks for every external program minimega calls, verifies minimum
versions, and confirms Open vSwitch is running. It prints nothing when all is
well and an error for each thing that is missing:

```minimega
minimega$ check
```

A missing program only breaks the features that use it, but the course needs
the whole set: QEMU for VMs, Open vSwitch for networks, dnsmasq for the host
DHCP server in chapter 6. Fix anything `check` reports before going on. The
most common report on RHEL-family hosts is the missing `kvm` name described
above; the most common on any host is Open vSwitch not started.

`help` lists every command with a one-line summary, and `help <command>`
prints the full text with the exact patterns a command accepts:

```minimega
minimega$ help
minimega$ help vm launch
```

The prompt completes command words with Tab and keeps history, so
`help` plus Tab is a fast way to explore.

## Commands from the shell

Everything you type at the prompt can also be sent from a shell. `-e` runs
one command on the daemon, prints the answer and exits, which is what scripts
and the rest of this course use for quick checks:

```bash
sudo minimega -e version
sudo minimega -e check
sudo minimega -e .columns name,state vm info
```

From the Docker route, prefix these with `docker exec minimega`, or use the
container's `mm` shortcut: `docker exec minimega mm vm info`.

Under the packaged service, members of the `minimega` group can attach and
run `-e` without `sudo`. Add yourself and log in again:

```bash
sudo usermod -aG minimega "$USER"
```

Being in that group is equivalent to root on the host, because minimega runs
as root and will run anything you ask; see
[Security considerations](../../articles/security.md).

Tab completion for the `minimega` command itself, including the minimega
commands after `-e`, is generated by the binary. The packages and the Docker
image install it for bash already; for the current shell only:

```bash
source <(minimega -completion bash)
```

`-completion zsh` and `-completion fish` produce the equivalents for those
shells; see [Running minimega](../../articles/running.md#shell-completion) for
where to install them.

## Two host services that get in the way

Both of these bite later, when the first network exists, so fix them now.

Installing the `dnsmasq` package on Debian and Ubuntu enables a system-wide
dnsmasq listening on every interface. minimega starts its own instances on
demand, and they fail to bind while the system one holds the port. minimega
only needs the binary:

```bash
sudo systemctl disable --now dnsmasq
```

On desktop hosts NetworkManager may take over the `mega_bridge` and
`mega_tap*` interfaces minimega creates. Tell it not to, as described under
[services that fight over the bridge](../../articles/installing.md#services-that-fight-over-the-bridge).

## Stop, restart, and clean up

`quit` at the prompt stops the daemon, which destroys every VM, tap and
bridge it created. On an attached prompt you type it twice. Under systemd a
clean exit is restarted by the unit (`Restart=on-success`), so to keep the
service down use `systemctl`:

```bash
sudo systemctl stop minimega       # package
docker stop minimega               # Docker
```

A source build in the foreground stops with `quit` or Ctrl-C.

If a daemon crashes, or is killed, it can leave QEMU processes, taps and the
`mega_bridge` behind, and the next start refuses to run over the stale
socket: `minimega appears to already be running, override with -force`.
Start once with `-force` and run `nuke`:

```bash
sudo minimega -force -nostdin &
sudo minimega -e nuke
```

Under the packaged service `MM_FORCE` is already `true` in
`minimega.conf`, so `sudo systemctl start minimega` followed by
`sudo minimega -e nuke` does the same.

`nuke` kills the QEMU and dnsmasq processes recorded under the base
directory, deletes every `mega_tap*` interface, removes the bridges minimega
created, deletes the base directory, and exits. It is the sledgehammer; on a
healthy daemon `clear all` resets state without stopping the process. When
the old daemon's VMs are still running and you want them back rather than
gone, `-recover` adopts them instead; see
[Recovering from a previous instance](../../articles/running.md#recovering-from-a-previous-instance).

## What you built

- minimega installed from a package, the Docker image, or source, with its
  runtime programs in place and Open vSwitch running.
- A daemon you can reach from any shell with `minimega -attach` and
  `minimega -e`.
- A host whose system dnsmasq and NetworkManager will not fight minimega over
  its bridge.

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

[Chapter 2: Get VM images](02-images.md) builds the two guest images the
course runs on.
