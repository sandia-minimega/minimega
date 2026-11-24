# File management

minimega serves a directory of files to every node in a mesh through iomeshage.
iomeshage is a transfer layer built on the same meshage protocol that carries
commands between nodes. A node that needs an image, a kernel, or a container
tarball asks the mesh for it by name. The node receives the file, in parts, from
any node that already has it. This can be faster than copying from a single
source. This page covers the files directory, the `file` API, and the `file:`
and `tar:` prefixes that fetch files inline. It also covers the flags
that resolve conflicts between different files with the same name on two nodes.

You need it on any [cluster](cluster.md). If you use the prefixes or the `disk`
API, you also need it on a single node. The `disk` API resolves relative paths
against the same directory. [Running minimega](running.md) describes the startup
flags.

## The files directory

Everything iomeshage can see lives under the directory given by `-filepath`,
`/tmp/minimega/files` by default. If you move minimega's base directory with
`-base` and do not set `-filepath`, the files directory follows it to
`<base>/files`. Paths in `file` commands, the `disk` API, and the prefixes are
relative to this directory. Permissions on transferred files are preserved.

## The file API

`file list` shows what the local node serves. With no arguments it lists the
top of the files directory. To list a subdirectory, give a path. To descend,
add `recursive`. You can also use a glob:

```minimega
minimega$ file list
host | dir   | name              | size      | modified
mm1  | <dir> | miniccc_responses | 4096      | 2026-09-14T10:02:11Z
mm1  |       | miniccc.kernel    | 7815424   | 2026-09-12T16:40:03Z
mm1  |       | miniccc.initrd    | 141852672 | 2026-09-12T16:40:09Z
minimega$ file list images recursive
minimega$ file list *.qc2
```

When minimega runs with `-hashfiles`, `file list` adds a `hash` column.

`file get` transfers a file, a glob, or a whole directory to the local files
directory. It returns as soon as the transfer is queued. If the file already
exists locally, the command returns immediately without a transfer. On a mesh
of one node, the command does nothing. `file status` shows transfers in
flight:

```minimega
minimega$ file get bigfile.qc2
minimega$ file status
host | filename     | tempdir                                | completed | queued
mm1  | bigfile.qc2  | /tmp/minimega/files/transfer_442933642 | 65/103    | false
```

`file delete` removes a file or, recursively, a directory from the local
node. It also takes globs. `file stream` returns the contents of a file in
parts as successive responses, without writing anything to disk. It blocks
until the last part. It is meant for programs that talk to minimega over the
command socket or the Python bindings. It is not meant for interactive use.

```minimega
minimega$ file delete *.iso
minimega$ file stream miniccc.kernel
```

Transfers are always pulls. There is no command to push a file to another
node. Instead, tell that node to fetch it:

```minimega
minimega$ mesh send mm2 file get bigfile.qc2
```

## Same name, different content

iomeshage identifies a file by its path under the files directory. If two
nodes each serve a `base.qc2` with different content, which one a third
node receives for `file get base.qc2` is undefined. Two startup flags fix
that:

- `-hashfiles` makes every node hash its files in the background. When a file
  is requested, the nodes compare hashes. When the copies on the mesh differ,
  the one with the newest modification time wins. A file is hashed once it has
  gone a couple of seconds without changing, so a large file being copied into the
  files directory is hashed once, when the copy finishes. Until then the file
  has no hash (the `hash` column of `file list` is empty).
- `-headnode <host>` names a node whose copies take precedence. A `file get`
  then compares the hash of the local copy with the hash of the head node. If
  they differ or the local copy is missing, it fetches from the head node. If
  the head node does not have the file, it keeps the local copy when one
  exists, and otherwise the normal search happens.

  `-headnode` requires `-hashfiles` on every node. Without `-hashfiles`,
  `file get` does not compare hashes. It fetches the file from the first node
  that answers, even when a local copy exists, and overwrites the local copy.

Add new and updated files on the head node, and run every node with both
flags. `deploy launch` does this for you. It starts the remote nodes with
`-headnode` set to the deploying host. It also passes the other flags of that
host to the remote nodes, including `-hashfiles`. A cluster that you deploy
from a node started with `-hashfiles` gets both flags.

## Fetching files inline

Two prefixes let any command that takes a path fetch the file first. Unlike
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
directory becomes the filesystem path.

### http:// and https://

The preprocessor also changes an argument that starts with `http://` or
`https://`. It replaces the URL with the path of the URL in the files directory.
In this release, minimega does not download the file. On a cluster, the command
fails with an error such as `get /images/x.qcow2: file not found`, unless a node
has a file at that path. To give a URL to a command such as `cc exec curl`, put
`.preprocess false` before the command. As an alternative, leave the scheme off.

## See also

- [Cluster setup](cluster.md)
- [Running minimega](running.md) for `-filepath`, `-hashfiles`, and `-headnode`
- [Disk images and the disk API](disk-images.md)
- [Building images with vmbetter](vmbetter.md) for producing the tarballs `tar:` expects
- Reference: [`file`](../reference/minimega.md#file)
