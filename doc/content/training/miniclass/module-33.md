# Module 33 - File Transfer Revisited

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-33-file-transfer-revisited/).

## Introduction

iomeshage is a distributed file transfer layer that provides a means to very quickly copy files between minimega nodes. By leveraging minimega's meshage message passing protocol, iomeshage can exceed transfer speeds obtained with one-to-one copying.

There are two ways to use iomeshage - through the `file` API and via an inline `file:` prefix available anywhere on the command line. In order for iomeshage to locate files on remote nodes, the files must be located in the `filepath` directory provided to minimega (by default `/tmp/minimega/files`).

iomeshage supports transferring single files, globs (wildcard files such as foo*), and entire directories. Permissions on transferred files are preserved.

**CAUTION**: iomeshage uses filenames (including the path) as the unique identifier for that file. For example, if two nodes have a file "foo", which is different on each node, iomeshage will have undefined behavior when transferring the file.

## file API

With multiple minimega servers running in the same namespace.

Simply copy files to `<base>/files` on a node in the mesh.

```
mkdir -p /tmp/minimega/files/vm/
cp test.qcow /tmp/minimega/files/vm/
cp test2.qcow /tmp/minimega/files/vm/
```

Files are transferred using the `get` command.

```
minimega$ file get test.qcow
```

When a `get` command is issued, the node will begin searching for a file matching the path and name within the mesh.

If the file exists, it will be transferred to the requesting node.

If multiple different files exist with the same name, the behavior is undefined.

When a file transfer begins, control will return to minimega while the transfer completes.

If a directory is specified, that directory will be recursively transferred to the node.

```
minimega$ file get vms/*.qcow
```

To list files currently being served, issue the list command with a directory relative to the served directory:

```
file list /foo
```

Issuing `file list /` will list the contents of the served directory.

Files can be deleted with the `delete` command:

```
file delete /foo
```

To see files that are currently being transferred, use the `file status` command:

```
file status
```

## file: Prefix

You can reference files from across the mesh namespace directly using:

```
vm config disk file:foo.qcow2
```

This will block until the file is transferred to the node locally. Once complete, the path will be replaced with local reference to the file:

```
minimega$ vm config disk file:foo.qcow2
minimega$ vm config disk
[/tmp/minimega/files/foo.qcow2]
minimega$
```

You can also use tab completion on the file name across the mesh.

## tar: Prefix

The `tar:` prefix fetches and untars tarballs via meshage, typically for use as a container filesystem:

```
minimega$ vm config filesystem tar:containerfs.tar.gz
minimega$ vm config filesystem
[/tmp/minimega/files/containerfs]
minimega$
```

If the tarball contains more than a single top-level directory, it will return an error since the filesystem path is set to the top-level directory inside the tarball. If the tarball resides outside of the iomeshage directory, minimega will still untar the tarball if it exists on the local node running the container to the same directory where the tarball resides.

## http://, https:// Prefix

Similar to the `file:` prefix, an HTTP(s) URL can be supplied anywhere minimega expects a file on disk. minimega will block while it downloads the file to the iomeshage directory. If the file already exists in iomeshage (on any node), the iomeshage version will be fetched instead of requesting the file from the URL. For example, to create a VM based on an Ubuntu cloud image:

```
minimega$ vm config disk https://uec-images.ubuntu.com/releases/14.04/release/ubuntu-14.04-server-cloudimg-amd64-disk1.img
minimega$ vm config disk
[/tmp/minimega/files/releases/14.04/release/ubuntu-14.04-server-cloudimg-amd64-disk1.img]
minimega$ file list
dir   | name              | size
<dir> | miniccc_responses | 40
<dir> | releases          | 60
minimega$ file list releases
dir   | name  | size
<dir> | 14.04 | 60
minimega$ file list releases/14.04
dir   | name    | size
<dir> | release | 60
minimega$ file list releases/14.04/release/
dir  | name                                         | size
     | ubuntu-14.04-server-cloudimg-amd64-disk1.img | 259785216
```

### Authors

The minimega authors

Created: 30 May 2017

Last updated: 3 June 2022
