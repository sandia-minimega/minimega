# Miniclass

Miniclass is the minimega training course. It teaches minimega by building one
experiment end to end and then extending it, one feature at a time. Every
chapter is written for the current minimega release and ends with a runnable
command script you can download.

The course has three parts.

**Part I, Getting started**, is the 101. It installs minimega, builds VM
images, and constructs the running example, a two-VM network joined by a
router, then adds command and control, saves and replays the experiment, and
shows how to troubleshoot it. Read it in order; each chapter assumes the one
before.

**Part II, Going deeper**, is a set of feature labs that extend the same
experiment: background traffic, packet capture and mirrors, VNC recording and
scripting, runtime changes to running VMs, plumbing, and namespaces. Read
these in any order after Part I.

**Part III, Other guest types and scale**, covers containers, Android VMs,
specialty KVM guests, multi-host clusters, automation through the Python
bindings, and testing and contributing.

An instructor syllabus at the end of the course sequences the chapters into a
one-day or two-day class.

## Prerequisites

- A Linux host with hardware virtualization, or a VM with nested
  virtualization enabled. See the
  [installation guide](../../articles/installing.md) for the full list of
  requirements and packages.
- Root access on that host. minimega manages kernel modules, bridges, and
  virtual machines and runs as root.
- About 20 GB of free disk for images built during the course.

## Conventions

minimega's interactive prompt shows the active namespace, for example
`minimega[minimega]$` or `minimega[sandwich]$`. Part I shortens it to
`minimega$`; later chapters show the full prompt where the namespace
matters. Commands typed in a host shell are shown with `$`. Downloadable scripts use
the `.mm` extension and can be run with `minimega -e read <file>` or from the
prompt with `read <file>`.

## The historical miniclass

An earlier, self-paced course of the same name was written for minimega 2.x.
It is preserved unmodified under
[Legacy → Historical miniclass](../../legacy/miniclass/index.md).
