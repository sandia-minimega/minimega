# Instructor syllabus

This page sequences the [miniclass](index.md) chapters into a one-day or a
two-day class. It lists the time blocks, three to five hands-on labs, and
the hardware that each lab needs. It also lists an image checklist to work
through before the first day and what to stage on the lab hosts. Finally, it
lists the mistakes that cost the most class time. It is written for the
instructor.
Students read the chapters.

The course builds one experiment, the router sandwich of
[Chapter 3](03-router-sandwich.md). Every later chapter extends it, so the
schedule is strictly ordered on day one and mostly free on day two. Every
chapter except 1, 10, and 22 ends with a downloadable script that reproduces
its end state from a clean minimega. When a student falls behind, `read` the
script of the latest earlier chapter that has one, and continue. This is the
recovery path. The script of chapter 21 is Python, so run it as that chapter
shows.

## Audience and prerequisites

We recommend that students are comfortable in a Linux shell and have experience
with SSH. We also recommend that students know what a VLAN, a DHCP lease, and a
bridge are. No Go or Python is required. Chapter 21 is the only chapter with
code. Each student, or each pair, needs root on a lab host that meets the
requirements below. Assume nothing about what they know of QEMU or Open vSwitch.

## One-day class

Chapters 1 to 10 with three labs. Times assume a 09:00 start, an hour for
lunch, and two short breaks. The chapter blocks are demonstrations with
students typing along, and the labs are unassisted.

| Time | Block | Chapters | Notes |
|---|---|---|---|
| 09:00–09:30 | What minimega is, the course, and the running example | [Overview](index.md) | Show the sandwich diagram from Chapter 3 now and refer to it all day. |
| 09:30–10:15 | Install and run | [1](01-install.md) | Hosts are pre-staged. Students make sure that the install works with `check`, attach, `-e`, and `quit`. They also see the pre-flight checks fail on purpose on the instructor's host. |
| 10:15–10:45 | Images | [2](02-images.md) | Images are prebuilt (below). Walk through the vmbetter commands and the kernel/initrd pair. Start a real build in the background on the instructor's host. Show that it finishes after lunch. |
| 10:45–11:00 | Break | | |
| 11:00–12:00 | The router sandwich; **Lab A** | [3](03-router-sandwich.md) | Lab A: build the sandwich by hand. Make sure that it runs with `vm info`. Ping through the router with `cc exec`. Then `write` the session to a file and replay it after `clear namespace sandwich`. |
| 12:00–13:00 | Lunch | | |
| 13:00–13:45 | VMs | [4](04-vms.md) | Templates, states, ranges, `vm config save`, the other VM types in one paragraph each. |
| 13:45–14:15 | miniweb and VNC | [5](05-miniweb.md) | One miniweb per host. Students open their own. |
| 14:15–14:45 | Networking basics | [6](06-networking.md) | The host tap into `net_left`. See the shared-host pitfall below before choosing addresses. |
| 14:45–15:00 | Break | | |
| 15:00–15:30 | Routers | [7](07-routers.md) | Static leases and DNS on the sandwich router. If time is short, show the second router and OSPF as a demonstration. |
| 15:30–16:15 | Command and control, **Lab B** | [8](08-cc.md) | Lab B: use `cc exec`, `cc background`, `cc send`, and `cc recv` between the clients. Generate traffic with `ping` in the background and watch it in `vm top`. Add `qos add vm-left 0 loss 25` and watch it again. |
| 16:15–16:45 | Save, replay, clean up | [9](09-save-replay-cleanup.md) | `vm save` and `ns save` versus `vm config save`. Also `clear namespace` and `nuke`. |
| 16:45–17:30 | Troubleshooting, **Lab C** | [10](10-troubleshooting.md) | Lab C: students receive four broken copies of the Chapter 3 script. The faults are that the router never committed and that a VLAN alias is misspelled on one side. The other faults are an initrd path that does not exist and a host `dnsmasq` started on the same tap. Students fix each copy with `vm info`, `router router`, the log, and `check`. |
| 17:30 | Wrap-up | | Point at Part II and the two-day schedule. |

## Two-day class

Day one is the one-day class above. Day two adds the Part II labs, then
namespaces and clusters.

| Time | Block | Chapters | Notes |
|---|---|---|---|
| 09:00–09:15 | Recap and rebuild the sandwich from its script | [3](03-router-sandwich.md) | `read` the chapter script. Everyone starts from the same state. |
| 09:15–10:00 | Background traffic | [11](11-traffic.md) | protonuke server on `vm-left`, client on `vm-right`. |
| 10:00–10:45 | Capture and mirrors, **Lab D** | [12](12-capture.md) | Lab D: capture the traffic of `vm-left` to a PCAP. Add the `monitor` VM with a `tap mirror`. Read the PCAP with `tcpdump` on the host. Read the mirror capture with `cc exec tcpdump -r` on the monitor. |
| 10:45–11:00 | Break | | |
| 11:00–11:45 | VNC recording and scripting | [13](13-vnc.md) | Record a login on `vm-right`. Play it back on a second copy. |
| 11:45–12:15 | Runtime changes | [14](14-runtime-changes.md) | CD-ROM, USB hotplug, disconnecting the router's interfaces, `qos`. |
| 12:15–13:15 | Lunch | | |
| 13:15–14:00 | Namespaces | [16](16-namespaces.md) | A second copy of the sandwich in `sandwich2`. On a shared cluster, each student now moves into a namespace of their own. |
| 14:00–15:30 | Clusters, **Lab E** | [20](20-clusters.md) | Lab E, in pairs of hosts: form the mesh. Spread the sandwich over both hosts. Prove that the lease crossed the tunnel. Run `file list` on the second host. |
| 15:30–15:45 | Break | | |
| 15:45–16:30 | Electives | [17](17-containers.md), [18](18-android.md), [19](19-specialty-guests.md), [21](21-automation.md) | Pick one or two by audience. Use containers for scale, the Python bindings for automation, and Android or TPM for guest specialists. Unless you prepared the hosts for it (see below), each is a demonstration. |
| 16:30–17:00 | Testing, contributing, wrap-up | [22](22-testing-contributing.md) | Where the scripts of the course become tests. How to file a bug. |

[Chapter 15](15-plumbing.md) is not scheduled. It fits after Chapter 14
for a class that asks about inter-VM messaging. It takes about 30 minutes.

## Hardware per lab

Every lab host needs hardware virtualization and root. It also needs a
current Debian, Ubuntu, or RHEL-family amd64 install. Hardware
virtualization means Intel VT-x or AMD-V, or a VM with nested virtualization
enabled.

| Lab | Per student or pair | Notes |
|---|---|---|
| A, B, C (day 1) | One host: 4 cores, 12 GB RAM, 20 GB free disk | The sandwich is two 3 GB clients plus the router. Leave room for the host. |
| D (capture) | As above plus 3 GB RAM | The `monitor` VM is a third client image. |
| E (clusters) | Two hosts per pair, or a shared cluster | Hosts need unique hostnames, IP connectivity on port 9000 TCP and UDP, UDP port 4789 for the VXLAN tunnel, and SSH keys for `deploy`. Hosts also need either a second NIC on a VLAN-capable switch or plain IP reachability for the VXLAN tunnel. If nested virtualization is enabled in both VMs, two nested VMs on one workstation work. |
| Chapter 17 (containers) | Day-1 host, rebooted with cgroup v1 | `systemd.unified_cgroup_hierarchy=0` (plus `cgroup_enable=memory` on Debian-family) on the kernel command line. systemd 256 and 257 also need `SYSTEMD_CGROUP_ENABLE_LEGACY_FORCE=1`. systemd 258 and later cannot run this chapter. This needs a reboot, so decide before class. |
| Chapter 18 (Android) | Day-1 host plus 15 GB disk for the SDK and 4 GB RAM for the emulator | Avoid nested virtualization for this one. The emulator is slow under it. |
| Chapter 19 (TPM) | Day-1 host with the `swtpm` package | |

Miniweb runs on each lab host. Students need a browser that can reach it.

## Image prebuild checklist

Building images takes tens of minutes and a Debian mirror. So build the
images before class, once, on one machine. Then copy the results to every
lab host.

The images embed miniccc and minirouter. These must come from the same
minimega release as the daemon on the lab hosts. A mismatch logs
`mismatched miniccc version` at the warn level. The packaged
`MM_LOGLEVEL="error"` hides that message. A mismatch can drop or misreport
`cc` commands. When you upgrade minimega, rebuild the images.

Build from a source checkout of the release that you teach. Build
`bin/miniccc` and `bin/minirouter` first (`./scripts/build.bash`), because
the vmbetter overlays link to them:

| Files | Command | Needed by |
|---|---|---|
| `miniccc.kernel`, `miniccc.initrd` | `sudo bash misc/vmbetter.bash miniccc` | Every chapter from 3 on |
| `minirouter.kernel`, `minirouter.initrd` | `sudo bash misc/vmbetter.bash minirouter` | Every chapter from 3 on |
| `minicccfs.tar.gz` (container filesystem) | `sudo bash misc/vmbetter.bash minicccfs` | Chapter 17 |
| An Android SDK and an AVD named `Pixel_9a` | `sdkmanager` and `avdmanager`, as in [Chapter 18](18-android.md#prepare-the-host) | Chapter 18 |

The wrapper pins Debian `stable` and the US mirror. `stable` is now trixie,
and trixie fails to build. Change `branch=stable` to `branch=bookworm` in
`misc/vmbetter.bash`. If you need another mirror, change the mirror too. Look
in Chapters 11 to 14 for any additional media that they use (for example, an
ISO for `vm cdrom`). Stage that media too. Boot each image once on the build
machine before you copy it anywhere.

## What to stage on every lab host

1. minimega installed from the `.deb` or `.rpm`, with the service enabled and
   running with the defaults (`MM_DEGREE=0`). Day-one hosts must not form a
   mesh by accident. For Lab E, decide the `MM_CONTEXT` for each pair in
   advance and leave it unset until the lab.
2. `check` passes at the prompt: QEMU, Open vSwitch, dnsmasq, and the
   other tools are present at the versions minimega wants.
3. The `kvm` module loaded and the host's own `dnsmasq` service disabled,
   so that it cannot claim the bridge or the DHCP ports.
4. The images in `/tmp/minimega/files/`. Most distributions clear `/tmp` at
   boot. So also keep a copy under a persistent path, such as
   `/srv/minimega/images/`, and a one-line script to restore the images.
   Alternatively, set `MM_FILEPATH` to the persistent directory in
   `/etc/minimega/minimega.conf`.
5. The chapter scripts at an absolute path, for example
   `/srv/miniclass/scripts/`. The reason is that `read` resolves relative
   paths against the working directory of the daemon. Under systemd, this
   directory is `/`.
6. miniweb, from the same package, and a note of its port for the
   students.
7. For the electives:
    - cgroup v1 boot parameters (Chapter 17).
    - The Android SDK and AVD as root (Chapter 18).
    - `swtpm` (Chapter 19).
    - The Python module that matches the release (Chapter 21).
    - A source checkout that you built with `./scripts/build.bash`, for
      minitest (Chapter 22).
8. A run of the Chapter 3 script on each host, by you, the day before.

## Classroom pitfalls

Students who run the lab host as a VM on their laptop need nested
virtualization turned on in the outer hypervisor. Without it, `check`
passes, but every VM sits in `BUILDING` or `ERROR` with a KVM error in the
`error` tag. Chapter 1's pre-flight checks catch this, so make them the
first thing that everyone runs. When nested, boots are several times slower.
So lengthen the `shell sleep` waits in the scripts.

If several students work on one host, they share `mega_bridge`, the VLAN
pool, tap names, host addresses, and root. Give each student a namespace
(`namespace <name>`, `minimega -attach -namespace
<name>`, `/<name>/vms` in miniweb): VLAN aliases and `vm config` templates
are per namespace, so two students can both build `sandwich` without their
`net_left` networks touching. Bridges and bridge captures are not
namespaced, and tap names are global.

For Chapter 6, hand out distinct subnets for the host tap, or do that
chapter on separate hosts. Two taps with `10.0.0.0/24` addresses on one host
leave the routing table of the host ambiguous. `nuke`,
`clear namespace all`, and `clear all` act on the whole host, and they will
end the lab of everyone. Say so before Chapter 9.

On a shared cluster, minimega allocates VLAN aliases from the `-vlanrange`
pool (`MM_VLANRANGE`, `101-4096` by default), mesh-wide. If the cluster
trunks to a real switch, the switch must carry every tag in the pool. Other
traffic on that switch must not use those tags.

Before you launch any VM in a namespace, give the namespace its own slice
with `vlans range <min> <max>`. The ranges must not overlap. Alternatively,
narrow the pool for the whole cluster with `MM_VLANRANGE`. `vlans` shows
which tags minimega handed out.

For Lab E, you have two options. In the first option, every pair gets its
own `MM_CONTEXT` and forms a mesh of two. In the second option, the whole
room forms one mesh. Each pair works in its own namespace and uses
`ns add-hosts` to name only their two hosts. The second option is closer to
real use and lets students see `mesh status` report the whole room. The
first option isolates mistakes. Do not mix the options. A pair that types
the context of the room joins the mesh of the room.

Images built from one release and a daemon from another produce silent `cc`
failures. Compare `version` on the hosts and the build machine before you
copy images.

Three activities cost the most time:

- Image builds. Do them before class.
- The first boot of an Android emulator. It takes minutes.
- Students who type long `router` lines by hand. Have them `read` the script
  and edit it.

Keep the script of the previous chapter ready as the reset button, and keep
one spare host.

## See also

- [Miniclass overview](index.md)
- [Installation](../../articles/installing.md) and
  [Running minimega](../../articles/running.md) for the service
  configuration.
- [Namespaces](../../articles/namespaces.md) and
  [Cluster setup](../../articles/cluster.md) for shared-host and cluster
  classes.
- [Building images with vmbetter](../../articles/vmbetter.md)
