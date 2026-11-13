# File management

minimega serves a directory of files to every node in a mesh through
iomeshage, a transfer layer built on the same meshage protocol that carries
commands between nodes. A node that needs an image, a kernel, or a container
tarball asks the mesh for it by name and receives it, in parts, from whichever
nodes already have it, which can be faster than copying from a single source.
This page covers the files directory, the `file` API, the `file:`, `tar:`, and
`http://` prefixes that fetch files inline, and the flags that resolve
conflicts when two nodes hold different files under the same name.

You need it on any [cluster](cluster.md), and on a single node whenever you
use the prefixes or the `disk` API, which resolves relative paths against the
same directory. Startup flags are described in [Running minimega](running.md).

## The files directory

Everything iomeshage can see lives under the directory given by `-filepath`,
`/tmp/minimega/files` by default. If you move minimega's base directory with
`-base` and do not set `-filepath`, the files directory follows it to
`<base>/files`. Paths in `file` commands, the `disk` API, and the prefixes are
relative to this directory. Permissions on transferred files are preserved.

## The file API

`file list` shows what the local node serves. With no arguments it lists the
top of the files directory; give a path to list a subdirectory, add
`recursive` to descend, or use a glob:

```minimega
minimega$ file list
host | dir   | name              | size      | modified
mm1  | <dir> | miniccc_responses | 4096      | 2026-09-14T10:02:11Z
mm1  |       | miniccc.kernel    | 7815424   | 2026-09-12T16:40:03Z
mm1  |       | miniccc.initrd    | 141852672 | 2026-09-12T16:40:09Z
minimega$ file list images recursive
minimega$ file list *.qc2
```

When minimega runs with `-hashfiles`, a `hash` column is added.

`file get` transfers a file, a glob, or a whole directory to the local files
directory. It returns as soon as the transfer is queued; a file that already
exists locally returns immediately without a transfer, and on a mesh of one
node the command is a no-op. `file status` shows transfers in flight:

```minimega
minimega$ file get bigfile.qc2
minimega$ file status
host | filename     | tempdir                                | completed | queued
mm1  | bigfile.qc2  | /tmp/minimega/files/transfer_442933642 | 65/103    | false
```

`file delete` removes a file or, recursively, a directory from the local
node; it also takes globs. `file stream` returns a file's contents in parts as
successive responses, without writing anything to disk, and blocks until the
last part; it is meant for programs talking to minimega over the command
socket or the Python bindings rather than for interactive use.

```minimega
minimega$ file delete *.iso
minimega$ file stream miniccc.kernel
```

Transfers are always pulls. There is no command to push a file to another
node; instead tell that node to fetch it:

```minimega
minimega$ mesh send mm2 file get bigfile.qc2
```

## Same name, different content

iomeshage identifies a file by its path under the files directory. If two
nodes each serve a `base.qc2` with different content, which one a third
node receives for `file get base.qc2` is undefined. Two startup flags fix
that:

- `-hashfiles` makes every node hash its files in the background and compare
  hashes when a file is requested. When the copies on the mesh differ, the one
  with the newest modification time wins.
- `-headnode <host>` names a node whose copies take precedence. A `file get`
  then compares the local copy's hash with the head node's and fetches from
  the head node whenever they differ or the local copy is missing; if the head
  node does not have the file, the local copy is kept if present, and the
  normal search happens otherwise. `-headnode` requires `-hashfiles` on
  every node: without hashing both hashes are empty and compare equal, so
  `file get` treats the local copy as current and skips the transfer even
  when the file does not exist locally.

So add new and updated files on the head node, and run every node with both
flags. `deploy launch` does this for you: it starts the remote nodes with
`-headnode` set to the deploying host and passes along that host's other
flags, including `-hashfiles`, so a cluster deployed from a node started with
`-hashfiles` gets both.

## Fetching files inline

Three prefixes let any command that takes a path fetch the file first. Unlike
`file get`, they block until the transfer completes and then substitute the
local path.

### file:

```minimega
minimega$ vm config disks file:role.qc2
minimega$ vm config disks
[/tmp/minimega/files/role.qc2]
```

A qcow2 image with a backing file pulls its backing chain as well. `file:`
paths tab-complete across the mesh.

### tar:

`tar:` fetches a tarball and unpacks it, which is the usual way to distribute
a container filesystem:

```minimega
minimega$ vm config filesystem tar:minicccfs.tar.gz
minimega$ vm config filesystem
/tmp/minimega/files/minicccfs
```

The tarball must contain exactly one top-level directory, because that
directory becomes the filesystem path. A tarball given by an absolute path
outside the files directory is unpacked next to itself on the local node.

### http:// and https://

A URL is downloaded into the files directory under the URL's path, unless a
file at that path already exists somewhere on the mesh, in which case the mesh
copy is fetched instead. For example, to boot a Debian cloud image:

```minimega
minimega$ vm config disks https://cloud.debian.org/images/cloud/trixie/latest/debian-13-generic-amd64.qcow2
minimega$ vm config disks
[/tmp/minimega/files/images/cloud/trixie/latest/debian-13-generic-amd64.qcow2]
minimega$ file list images/cloud/trixie/latest
host | dir | name                            | size      | modified
mm1  |     | debian-13-generic-amd64.qcow2 | 396361728 | 2026-09-14T10:15:42Z
```

## See also

- [Cluster setup](cluster.md)
- [Running minimega](running.md) for `-filepath`, `-hashfiles`, and `-headnode`
- [Disk images and the disk API](disk-images.md)
- [Building images with vmbetter](vmbetter.md) for producing the tarballs `tar:` expects
- Reference: [`file`](../reference/minimega.md#file)
