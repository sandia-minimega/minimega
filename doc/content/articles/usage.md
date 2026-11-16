# Using minimega

This page used to be one long guide to starting minimega, typing commands,
describing VMs, networking them and joining hosts into a cluster. That material
now lives on focused pages; the ones below cover everything it did, with
current command syntax and output.

- [Running minimega](running.md): starting the daemon as a service or by hand,
  every startup flag and `MM_*` variable, attaching to a running instance,
  recovery, logging and shutdown.
- [Command line and scripting](cli.md): the prompt, output builtins such as
  `.columns` and `.filter`, command files, `minimega -e`, `shell` and
  `background`, and the command socket.
- [VM lifecycle](vm-lifecycle.md): describing, launching, starting, stopping
  and removing VMs.
- [Host networking](networking.md): VLANs, host taps, dnsmasq and NAT.
- [Cluster setup](cluster.md): meshage, `deploy` and multi-host operation.
