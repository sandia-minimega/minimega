# Historical miniclass

This self-paced course was recovered from the former Sandia minimega website.
It was written between 2016 and 2018 against minimega 2.2 through 2.7, Ubuntu
16.04, Python 2, and third-party tools that have since changed. It is kept for
reference only and is not maintained.

The current training course is the [miniclass](../../training/miniclass/index.md).
The current reference material is in the user guides linked from the
[documentation home page](../../index.md). Prefer those and the generated
[API reference](../../reference/minimega.md) wherever this material conflicts.

## What is kept and why

Of the original 53 modules, everything that was still true was merged into
the current user guides and the current course, and the modules that
duplicated a current page were removed. Git history retains every original
page. The seven modules below remain because they describe something no
current page covers, usually because the software or workflow is obsolete.

| Module | Why it is kept |
|---|---|
| [Module 1 - Installing A Base OS](module-01.md) | Ubuntu Server 16.04 installer walkthrough with screenshots. |
| [Module 5 - Starting and Stopping Minimega](module-05.md) | Launch-script and tmux era operations, before packages and the systemd unit. |
| [Module 7 - File Transfer](module-07.md) | Host-side file transfer tools (scp, rsync, WinSCP) for moving images to a host. |
| [Module 13 - Building a VM with virtualbox](module-13.md) | Building a guest in VirtualBox and exporting it for minimega. |
| [Module 25 - Xvesa Issues](module-25.md) | Display fixes for TinyCore Linux guests. |
| [Module 38 - Guacamole VNC Management](module-38.md) | Per-user VNC access through Apache Guacamole. |
| [Module 43 - Physical to Virtual](module-43.md) | Converting physical machines with Clonezilla and VMware Converter. |

Where the removed modules' material now lives:

- Host preparation, nested virtualization, kernel modules: [Installation](../../articles/installing.md).
- Starting VMs, multiple VMs, pausing, scripting: [VM lifecycle](../../articles/vm-lifecycle.md) and [Command line and scripting](../../articles/cli.md).
- Building images, injecting files, QMP: [Building images with vmbetter](../../articles/vmbetter.md), [Disk images and the disk API](../../articles/disk-images.md), [Virtual machine types](../../articles/vmtypes.md).
- Windows guests and miniccc: [Windows guests](../../articles/windows.md) and [Command and control](../../articles/cc.md).
- Static addressing, dnsmasq, taps and bridges, NAT, VLAN aliases: [Host networking](../../articles/networking.md).
- minirouter and the large OSPF example: [Routing with minirouter](../../articles/router.md).
- Containers, TPM, copy and paste: [Virtual machine types](../../articles/vmtypes.md).
- Plumbing: [Plumbing](../../articles/plumbing.md).
- Multiple nodes and meshage: [Cluster setup](../../articles/cluster.md).
- VNC recording, playback, scripting, vncdrone: [VNC](../../articles/vnc.md).
- Network capture: [Capture and instrumentation](../../articles/capture.md).
- Troubleshooting: [Troubleshooting](../../articles/troubleshooting.md).
- Namespaces, Python bindings, minitest, contributing: [Namespaces](../../articles/namespaces.md), [Python API](../../articles/python.md), [minitest](../../articles/minitest.md), [Contributing](../../articles/contributing.md).

Module 24 (VyOS) was never recovered, and modules 47 and 48 never existed on
the original site.
