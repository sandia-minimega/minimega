# Instructor syllabus

This page sequences the [miniclass](index.md) chapters into a one-day or a
two-day class, with time blocks, three to five hands-on labs, the hardware
each lab needs, an image checklist to work through before the first day,
what to stage on the lab hosts, and the mistakes that cost the most class
time. It is written for the instructor; students read the chapters.

The course builds one experiment, the router sandwich of
[Chapter 3](03-router-sandwich.md), and every later chapter extends it, so
the schedule is strictly ordered on day one and mostly free on day two.
Every chapter ends with a downloadable script that reproduces its end
state from a clean minimega, which is the recovery path whenever a student
falls behind: `read` the previous chapter's script and continue.

## Audience and prerequisites

Students should be comfortable in a Linux shell, know what a VLAN, a DHCP
lease, and a bridge are, and have used SSH. No Go or Python is required;
Chapter 21 is the only one with code. Each student, or each pair, needs
root on a lab host that meets the requirements below. Assume nothing about
what they know of QEMU or Open vSwitch.

## One-day class

Chapters 1 to 10 with three labs. Times assume a 09:00 start, an hour for
lunch, and two short breaks; the chapter blocks are demonstrations with
students typing along, and the labs are unassisted.

| Time | Block | Chapters | Notes |
|---|---|---|---|
| 09:00–09:30 | What minimega is; the course; the running example | [Overview](index.md) | Show the sandwich diagram from Chapter 3 now and refer to it all day. |
| 09:30–10:15 | Install and run | [1](01-install.md) | Hosts are pre-staged; students verify with `check`, attach, `-e`, and `quit`, and see the pre-flight checks fail on purpose on the instructor's host. |
| 10:15–10:45 | Images | [2](02-images.md) | Images are prebuilt (below). Walk through the vmbetter commands and the kernel/initrd pair; start a real build in the background on the instructor's host and show it finishing after lunch. |
| 10:45–11:00 | Break | | |
| 11:00–12:00 | The router sandwich; **Lab A** | [3](03-router-sandwich.md) | Lab A: build the sandwich by hand, verify with `vm info`, ping through the router with `cc exec`, then `write` the session to a file and replay it after `clear namespace sandwich`. |
| 12:00–13:00 | Lunch | | |
| 13:00–13:45 | VMs | [4](04-vms.md) | Templates, states, ranges, `vm config save`, the other VM types in one paragraph each. |
| 13:45–14:15 | miniweb and VNC | [5](05-miniweb.md) | One miniweb per host; students open their own. |
| 14:15–14:45 | Networking basics | [6](06-networking.md) | The host tap into `net_left`. See the shared-host pitfall below before choosing addresses. |
| 14:45–15:00 | Break | | |
| 15:00–15:30 | Routers | [7](07-routers.md) | Static leases and DNS on the sandwich router; the second router and OSPF as a demonstration if time is short. |
| 15:30–16:15 | Command and control; **Lab B** | [8](08-cc.md) | Lab B: `cc exec`, `cc background`, `cc send` and `cc recv` between the clients; generate traffic with `ping` in the background and watch it in `vm top`; add `qos add vm_left 0 loss 25` and watch it again. |
| 16:15–16:45 | Save, replay, clean up | [9](09-save-replay-cleanup.md) | `vm save` and `ns save` versus `vm config save`; `clear namespace`; `nuke`. |
| 16:45–17:30 | Troubleshooting; **Lab C** | [10](10-troubleshooting.md) | Lab C: students receive four broken copies of the Chapter 3 script (router never committed, a VLAN alias misspelt on one side, an initrd path that does not exist, a host `dnsmasq` started on the same tap) and fix each using `vm info`, `router router`, the log, and `check`. |
| 17:30 | Wrap-up | | Point at Part II and the two-day schedule. |

## Two-day class

Day one is the one-day class above. Day two adds the Part II labs, then
namespaces and clusters.

| Time | Block | Chapters | Notes |
|---|---|---|---|
| 09:00–09:15 | Recap; rebuild the sandwich from its script | [3](03-router-sandwich.md) | `read` the chapter script; everyone starts from the same state. |
| 09:15–10:00 | Background traffic | [11](11-traffic.md) | protonuke server on `vm_left`, client on `vm_right`. |
| 10:00–10:45 | Capture and mirrors; **Lab D** | [12](12-capture.md) | Lab D: capture `vm_left`'s traffic to a PCAP, add the `monitor` VM with a `tap mirror`, and read both captures with `tcpdump` on the host. |
| 10:45–11:00 | Break | | |
| 11:00–11:45 | VNC recording and scripting | [13](13-vnc.md) | Record a login on `vm_right`, play it back on a second copy. |
| 11:45–12:15 | Runtime changes | [14](14-runtime-changes.md) | CD-ROM, USB hotplug, disconnecting the router's interfaces, `qos`. |
| 12:15–13:15 | Lunch | | |
| 13:15–14:00 | Namespaces | [16](16-namespaces.md) | A second copy of the sandwich in `sandwich2`; on a shared cluster, each student now moves into a namespace of their own. |
| 14:00–15:30 | Clusters; **Lab E** | [20](20-clusters.md) | Lab E, in pairs of hosts: form the mesh, spread the sandwich over both hosts, prove the lease crossed the tunnel, `file list` on the second host. |
| 15:30–15:45 | Break | | |
| 15:45–16:30 | Electives | [17](17-containers.md), [18](18-android.md), [19](19-specialty-guests.md), [21](21-automation.md) | Pick one or two by audience: containers for scale, the Python bindings for automation, Android or TPM for guest specialists. Each is a demonstration unless the hosts were prepared for it (see below). |
| 16:30–17:00 | Testing, contributing, wrap-up | [22](22-testing-contributing.md) | Where the course's own scripts become tests; how to file a bug. |

[Chapter 15](15-plumbing.md) is not scheduled; it fits after Chapter 14
for a class that has asked about inter-VM messaging, and takes about 30
minutes.

## Hardware per lab

Every lab host needs hardware virtualization (Intel VT-x or AMD-V, or a
VM with nested virtualization enabled), a current Debian, Ubuntu, or
RHEL-family amd64 install, and root.

| Lab | Per student or pair | Notes |
|---|---|---|
| A, B, C (day 1) | One host: 4 cores, 8 GB RAM, 20 GB free disk | The sandwich is two 2 GB clients plus the router; leave room for the host. |
| D (capture) | As above plus 2 GB RAM | The `monitor` VM is a third client image. |
| E (clusters) | Two hosts per pair, or a shared cluster | Hosts need unique hostnames, IP connectivity on port 9000 TCP and UDP, SSH keys for `deploy`, and either a second NIC on a VLAN-capable switch or plain IP reachability for the VXLAN tunnel. Two nested VMs on one workstation work if both have nested virtualization. |
| Chapter 17 (containers) | Day-1 host, rebooted with cgroup v1 | `systemd.unified_cgroup_hierarchy=0` (plus `cgroup_enable=memory` on Debian-family) on the kernel command line; a reboot, so decide before class. |
| Chapter 18 (Android) | Day-1 host plus 15 GB disk for the SDK and 4 GB RAM for the emulator | Avoid nested virtualization for this one; the emulator is slow under it. |
| Chapter 19 (TPM) | Day-1 host with the `swtpm` package | |

Miniweb runs on each lab host; students need a browser that can reach it.

## Image prebuild checklist

Building images takes tens of minutes and a Debian mirror, so it is done
before class, once, on one machine, and the results are copied to every
lab host. The images embed miniccc and minirouter, which must come from the
same minimega release as the daemon on the lab hosts; a mismatch shows up
as `mismatched miniccc version` in the log and missing `cc` clients, so
rebuild the images whenever you upgrade minimega.

Build from a source checkout of the release you are teaching, with
`bin/miniccc` and `bin/minirouter` built (`./scripts/build.bash`), because
the vmbetter overlays link to them:

| Files | Command | Needed by |
|---|---|---|
| `miniccc.kernel`, `miniccc.initrd` | `sudo bash misc/vmbetter.bash miniccc` | Every chapter from 3 on |
| `minirouter.kernel`, `minirouter.initrd` | `sudo bash misc/vmbetter.bash minirouter` | Every chapter from 3 on |
| `minicccfs.tar.gz` (container filesystem) | `sudo bash misc/vmbetter.bash minicccfs` | Chapter 17 |
| An Android SDK and an AVD named `Pixel_9a` | `sdkmanager` and `avdmanager`, as in [Chapter 18](18-android.md#prepare-the-host) | Chapter 18 |

The wrapper pins Debian `stable` and the US mirror; edit `misc/vmbetter.bash`
if you need another mirror. Check Chapters 11 to 14 for any additional
media they use (an ISO for `vm cdrom`, for example) and stage that too.
Boot each image once on the build machine before copying it anywhere.

## What to stage on every lab host

1. minimega installed from the `.deb` or `.rpm`, the service enabled and
   running with the defaults (`MM_DEGREE=0`; day-one hosts must not form a
   mesh by accident). For Lab E, decide the `MM_CONTEXT` per pair in
   advance and leave it unset until the lab.
2. `check` passes at the prompt: QEMU, Open vSwitch, dnsmasq, and the
   other tools are present at the versions minimega wants.
3. The `kvm` module loaded and the host's own `dnsmasq` service disabled,
   so that it cannot claim the bridge or the DHCP ports.
4. The images in `/tmp/minimega/files/`. `/tmp` is cleared at boot on most
   distributions, so also keep a copy under a persistent path such as
   `/srv/minimega/images/` and a one-line script to restore them, or set
   `MM_FILEPATH` to the persistent directory in
   `/etc/minimega/minimega.conf`.
5. The chapter scripts at an absolute path, for example
   `/srv/miniclass/scripts/`, because `read` resolves relative paths against
   the daemon's working directory, which is `/` under systemd.
6. miniweb, from the same package, and a note of its port for the
   students.
7. For the electives: cgroup v1 boot parameters (Chapter 17), the Android
   SDK and AVD as root (Chapter 18), `swtpm` (Chapter 19), the Python module
   matching the release (Chapter 21), and a source checkout built with
   `./scripts/build.bash` for minitest (Chapter 22).
8. A run of the Chapter 3 script on each host, by you, the day before.

## Classroom pitfalls

**Nested virtualization.** Students running the lab host as a VM on their
laptop need nested virtualization turned on in the outer hypervisor, or
`check` passes and every VM sits in `BUILDING` or `ERROR` with a KVM error
in the `error` tag. Chapter 1's pre-flight checks catch this; make it the
first thing everyone runs. Boots are several times slower nested, so
lengthen the `shell sleep` waits in the scripts.

**Shared hosts share the bridge.** If several students work on one host,
they share `mega_bridge`, the VLAN pool, host taps, and root. Give each
student a namespace (`namespace <name>`, `minimega -attach -namespace
<name>`, `/<name>/vms` in miniweb): VLAN aliases and `vm config` templates
are per namespace, so two students can both build `sandwich` without their
`net_left` networks touching. Host taps and bridge captures are not
namespaced. For Chapter 6, hand out distinct subnets for the host tap, or
do that chapter on separate hosts, because two taps with `10.0.0.0/24`
addresses on one host leave the host's routing table ambiguous. `nuke`,
`clear namespace all`, and `clear all` act on the whole host and will end
everyone's lab; say so before Chapter 9.

**VLAN ranges on a shared cluster.** Aliases are allocated from the
`-vlanrange` pool (`MM_VLANRANGE`, `101-4096` by default), mesh-wide. On a
cluster that trunks to a real switch, the switch has to carry every tag in
the pool, and other traffic on that switch must not use those tags. Give
each namespace its own slice with `vlans range <min> <max>` before any VM
is launched in it (ranges may not overlap), or narrow the pool for the
whole cluster with `MM_VLANRANGE`. `vlans` shows what has been handed out.

**One mesh, or several.** For Lab E, either every pair gets its own
`MM_CONTEXT` and forms a mesh of two, or the whole room forms one mesh and
each pair works in its own namespace with `ns add-hosts` naming just their
two hosts. The second is closer to real use and lets students see
`mesh status` report the whole room; the first isolates mistakes. Do not
mix: a pair that types the room's context joins the room's mesh.

**Version mismatches.** Images built from one release and a daemon from
another produce silent `cc` failures. Check `version` on the hosts and the
build machine against each other before you copy images.

**Time sinks.** Image builds (do them before), the first boot of an
Android emulator (minutes), and students typing long `router` lines by
hand (have them `read` the script and edit it). Keep the previous
chapter's script ready as the reset button, and keep one spare host.

## See also

- [Miniclass overview](index.md)
- [Installation](../../articles/installing.md) and
  [Running minimega](../../articles/running.md) for the service
  configuration.
- [Namespaces](../../articles/namespaces.md) and
  [Cluster setup](../../articles/cluster.md) for shared-host and cluster
  classes.
- [Building images with vmbetter](../../articles/vmbetter.md)
