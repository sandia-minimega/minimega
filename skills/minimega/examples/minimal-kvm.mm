# Minimal KVM command file for minimega.
#
# Paths below are relative to the minimega repository root; substitute the
# absolute path when running from elsewhere or from a symlinked skill install.
#
# Validate syntax without executing anything:
#   minimega -e read skills/minimega/examples/minimal-kvm.mm check
# Apply against a running instance (add -base=<path> first when the daemon
# uses a non-default base):
#   minimega -e read skills/minimega/examples/minimal-kvm.mm
#
# Requires a bootable disk image. A relative image path resolves beneath the
# daemon's -filepath (default /tmp/minimega/files); use an absolute path when
# the image lives elsewhere. `read` stops at invalid syntax, continues past a
# command that returns an error, and rejects nested `read` commands.

# Work in a dedicated namespace so cleanup is one command:
#   clear namespace example
namespace example

# Start from a known configuration; vm config persists across launches.
clear vm config
vm config memory 1024
vm config vcpus 1
vm config disk ubuntu.qcow2
vm config net LAN

# vm launch creates the VM in the BUILDING state; vm start boots it.
vm launch kvm node1
vm start node1

# Select columns explicitly instead of parsing the default table by position.
.columns name,state vm info
