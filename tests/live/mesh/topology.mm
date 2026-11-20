# minimega live integration test topology: a two-node mesh
#
# Four containers in one namespace that spans two minimega nodes, two
# scheduled on each, all using a filesystem from the tar: prefix. Only the
# first node has the tarball, so the second node must fetch it via iomeshage
# and extract it before launching (sandia-minimega/minimega#1396).
#
# Requirements (staged by tests/live/run.sh and tests/live/mesh/setup.sh):
#   - live/containerfs.tar.gz in the first node's files directory (-filepath)
#   - the first node is "minimega" and the second, already in the mesh, is
#     "minimega-peer" (used by "ns add-hosts" and "vm config schedule")
#
# Remove everything it creates with:
#   clear namespace live

namespace live

ns queueing true
ns add-hosts minimega,minimega-peer

clear vm config
vm config filesystem tar:live/containerfs.tar.gz
vm config init /bin/sleep infinity
vm config memory 64

vm config schedule minimega
vm launch container ct-head[1-2]

vm config schedule minimega-peer
vm launch container ct-peer[1-2]

vm launch
vm start all
