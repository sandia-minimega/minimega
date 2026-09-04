# Module 28 - MiniCCC

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-28-miniccc-and-the-cc-api/).

## Introduction

The Command and Control (`cc`) API in minimega provides a mechanism to programmatically execute programs and send and receive files to VMs launched by minimega. In addition, the `cc` API allows creating TCP tunnels (as well as reverse tunnels) from the host machine to/from any VM.

The `cc` API supports two modes of communication with VMs:

1. The first is a simple TCP connection from the VM to the host via some routable network. In this mode the VMs must be able to communicate directly with the host they are launched on. This can be accomplished with the `tap` API, or through several other means.
2. The second connection mode is over either `virtio-serial` (for KVM-based VMs), or over a UNIX domain socket (for container-based VMs), which provides a simple I/O layer, similar to a UNIX pipe, between the host and VM. In order to use the `virtio-serial` mode, both host and VM must have `virtio-serial` support. Most modern Linux distributions have this enabled by default. Windows VMs must install `virtio-serial` drivers, available from [here](http://www.linux-kvm.org/page/WindowsGuestDrivers/Download_Drivers). Domain sockets for container-based VMs are created by default when launching containers. Note that to use the `virtio` service, your VM and host both need to have `virtio` support available.

Both connection modes provide the same features, although the TCP mode can be significantly faster, depending on the underlying network.

Commands issued with the `cc` API (including file I/O) are always executed *in order* on the client. This means you can chain commands, such as "send a file, execute that file, then receive the output from that file."

Commands can also contain filters, which allow you to select which VMs execute that command. Filters can be stacked, meaning that you can apply filters such as "execute this command on Windows VMs with an IP in the network 10.0.0.0/24."

`miniccc` is a virtual machine agent written in Go that can communicate over IP or serial, transfer files, and execute commands. It facilitates communications initiated through calls to the `cc` API.

## Installation

This section walks through the process of setting up a VM with a working `miniccc` client. If you already have an existing `miniccc` image or just want to review the `cc` API itself, you can skip to the Commands section in this module.

### Windows 7

Build a Windows 7 VM

```
disk create qcow2 /home/ubuntu/w7.qcow 100G
vm config disk /home/ubuntu/w7.qcow
vm config cdrom /home/ubuntu/w7.iso
vm config snapshot false
vm config memory 1024
vm config net 100
vm launch kvm w7
vm start w7
```

### Install virtio for Windows 7

```
wget https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso
vm cdrom change w7 /home/ubuntu/virtio-win.iso
```

Open Device Manager

- Right click "PCI Simple Communications Controller"
- Update Driver Software
- Browse my computer
- `D:\vioserial\w7\`
- Install
- Repeat for the other adapter

Eject the ISO

```
vm cdrom eject w7
```

#### Building miniccc for x86 windows

At the end of `/home/ubuntu/minimega/build.bash` add

```
# build windows packages
echo "BUILD PACKAGES (windows)"
echo "protonuke"
GOOS=windows GOARCH=386 go install protonuke
if [[ $? != 0 ]]; then
        exit 1
fi
echo "miniccc"
GOOS=windows GOARCH=386 go install miniccc
if [[ $? != 0 ]]; then
        exit 1
fi
echo
unset GOOS
```

And run

```
/home/ubuntu/minimega/build.bash
```

#### Download miniccc

Create a tap so the VM can talk to the host via IP.

```
tap create 100 ip 10.0.0.1/24 mytap
```

Start a `dnsmasq` service so the VM gets an IP automatically

```
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
```

Start a webserver on the host so we can copy over the `miniccc` binary.

```
cd /home/ubuntu/minimega/bin/
python -m SimpleHTTPServer
```

With a browser in the virtual machine navigate to http://1.1.1.1:8000 and download `miniccc.exe` from either `windows_amd64/` or `windows_386/` depending on your installation.

#### Configuring Windows 7 to start miniccc

With Notepad create `mini.bat` file on the desktop with the following contents:

```
C:\miniccc.exe -serial \\.\Global\cc
```

The `miniccc` client uses several command line switches to control how to connect to minimega, as well as where to store files received for the client, which we'll see throughout this module. Here, we just used the `-serial` flag to connect using `virtio-serial`. In Windows, the default `virtio-serial` `cc` port is `\\.\Global\cc` ; note that this is the path we use in this batch file. Move this file to `C:\`

Open Task Scheduler

- Click 'Create Task'
- Name the task miniccc
- Select Run whether user is logged on or not
- Select Run with highest privileges
- Check Hidden
- Click on the Triggers Tab
- Click New
- Select Begin the Task: At startup
- Click Ok
- Click on the Actions Tab
- Click New
- Click Browse
- Select `C:\mini.bat`
- Click Ok
- Click Ok
- Click Ok
- Type in a username and password for an Administrative User.
- If the user doesn't have a password you will need to set one first

You can now turn off the virtual machine and delete the tap.

```
dnsmasq kill 0
tap delete mytap
```

#### Inject miniccc IP

```
mkdir -p /mnt/kvm
cp /home/ubuntu/win7.qcow /home/ubuntu/win7ip.qcow
qemu-nbd --connect /dev/nbd0 /home/ubuntu/win7ip.qcow
mount /dev/nbd0p2 /mnt/kvm
echo "C:\\miniccc.exe -parent 10.0.0.1" > /mnt/kvm/mini.bat
umount /mnt/kvm
qemu-nbd --disconnect /dev/nbd0
```

To configure `miniccc` to connect over TCP instead of serial, we have just used the `-parent` flag, providing the host/IP of host running minimega.

### Ubuntu Server

Build an Ubuntu 16.04 image as `snapshot false`

```
disk create qcow2 /home/ubuntu/u1604.qcow2 100G
vm config cdrom /home/ubuntu/u1604.iso
vm config disk /home/ubuntu/u1604.qcow2
vm config snapshot false
vm config memory 1024
vm config net 100
vm launch kvm u1604
vm start u1604
```

#### Inject miniccc

Using a serial connection:

```
cp /home/ubuntu/u1604.qcow2 /home/ubuntu/u1604ip.qcow2
qemu-nbd --connect /dev/nbd0 /home/ubuntu/u1604.qcow2
mount /dev/nbd0p1 /mnt/kvm
sed -i '$i/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /miniccc.log &' /mnt/kvm/etc/rc.local
cp /home/ubuntu/minimega/bin/miniccc /mnt/kvm/miniccc
umount /mnt/kvm
qemu-nbd --disconnect /dev/nbd0
```

In Linux, the default `virtio-serial` `cc` port is `/dev/virtio-ports/cc`, which differs from that of Windows (recall that the Windows path was `\\.\Global\cc` ). Nevertheless, as we previously did with the Windows VMs, we have now set the appropriate path when starting `miniccc` in our Ubuntu VMs over a serial connection.

Using a connection over IP:

```
qemu-nbd --connect /dev/nbd0 /home/ubuntu/u1604ip.qcow2
mount /dev/nbd0p1 /mnt/kvm
sed -i '$i/miniccc -v=false -parent 10.0.0.1 -logfile /miniccc.log &' /mnt/kvm/etc/rc.local
umount /mnt/kvm
qemu-nbd --disconnect /dev/nbd0
```

NOTE: By default, `miniccc` will create the directory `/tmp/miniccc` to store state and files in. Files sent to the client will be stored in `/tmp/miniccc/files`. You can change this directory with the `-base` flag.

## Launch VMs

Let's launch the VMs we created and give them IP addresses:

```
vm kill all
vm flush
clear vm config
vm config disk /home/ubuntu/u1604.qcow2
vm config memory 1024
vm launch kvm u1604
vm config disk /home/ubuntu/win7.qcow
vm launch kvm win7
vm config net 100
vm config disk /home/ubuntu/u1604ip.qcow2
vm launch kvm u1604ip
vm config disk /home/ubuntu/win7ip.qcow
vm config tag foo:bar
vm launch kvm win7ip
vm start all
tap create 100 ip 10.0.0.1/24 mytap
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
```

We are now ready to exercise the `cc` API, which we will do in the next section.

## Commands

This section describes the `cc` API in greater detail.

### Client Information

`cc` lists a count of the number of connected clients

```
$ cc
host   | clients
ubuntu | 4
```

`cc <tab>` shows all the commands available

```
background  commands    exec        log         process     responses   send
clients     delete      filter      prefix      recv        rtunnel     tunnel
```

`cc clients` lists the connected clients

```
$ cc clients
host   | uuid                                 | hostname        | arch  | os      | ip                                    | mac                 | namespace
ubuntu | 2220ebfc-d3bd-4caf-8c38-0666309dae5d | WIN-8QHRPBH7HPK | 386   | windows | [fe80::f0d3:56e4:bd6:2a3c 10.0.0.175] | [00:05:5d:96:af:ff] |
ubuntu | 79832887-a373-4bd8-bee7-50ba7e098b81 | WIN-8QHRPBH7HPK | 386   | windows | []                                    | []                  |
ubuntu | bc59214a-86aa-46cb-8810-a62dca455f9d | ubuntu          | amd64 | linux   | [10.0.0.67 fe80::c2c5:20ff:fe24:be1b] | [c0:c5:20:24:be:1b] |
ubuntu | fa3ce3a5-27f0-4eac-8937-72fe1da13722 | ubuntu          | amd64 | linux   | []                                    | []                  |
```

Clients report their UUID, hostname, OS, architecture, IP and MAC addresses to minimega. This information is updated periodically, so if an IP changes, minimega will see the change.

Client information is stored by UUID in minimega. When a client responds to a command, the response is logged by minimega in a subdirectory named after the UUID for that client. We'll discuss responses later.

### Executing Commands

Executing commands is simple - just issue `cc exec` with the command you want to execute:

```
# trying a Linux command on a Windows VM -- we'll see that this will fail
$ cc exec echo hello
```

When the client responds to a `cc exec` command, standard out and standard error are stored in the files `stdout` and `stderr` respectively.

In some cases, you may need to wrap a command in quotes or escape special characters. You can inspect current in-flight commands with `cc commands`, which shows the contents of the command, any applied filters (more on that later), and how many clients have responded:

```
# list all commands
$ cc commands
```

There are two things to note at this point. First, commands don't go away until you delete them with `cc delete command`. This means that if you were to reset a VM or start new VMs, they would all see and execute this command. Second, the response from the client isn't printed to screen. Instead, responses are logged in a special directory structure in minimega's base path. You can browse to the responses yourself, or just use the `cc responses` command to view responses from clients, like this:

```
$ cc responses all
ubuntu: 1/2220ebfc-d3bd-4caf-8c38-0666309dae5d/stderr:
exec: "echo": executable file not found in %PATH%
1/79832887-a373-4bd8-bee7-50ba7e098b81/stderr:
exec: "echo": executable file not found in %PATH%
1/bc59214a-86aa-46cb-8810-a62dca455f9d/stdout:
hello

1/fa3ce3a5-27f0-4eac-8937-72fe1da13722/stdout:
hello
```

As you can see Windows isn't too happy. It doesn't know where the `echo` executable binary is.

In all of the examples so far, each command is run by every connecting client. Sometimes you want to only send commands to specific clients, say, by hostname or IP range. The `cc filter` command allows you to set a client filter that will be applied to every command issued while the filter is assigned.

Let's set a filter for Windows

```
# first clear the buffer of responses
clear cc responses

# verify that responses have been cleared
cc responses all

# set a client filter on Windows hosts
cc filter os=windows

# and show the current filter
cc filter
```

Any commands issued from now on will be executed only by clients that meet all filter fields. You can set multiple filter arguments, such as restricting commands to only Windows machines in a specific IP range.

Now run the the same command again, this time with `cmd`

```
cc exec cmd /c 'echo hello'

$ cc responses all
2/2220ebfc-d3bd-4caf-8c38-0666309dae5d/stdout:
hello

2/79832887-a373-4bd8-bee7-50ba7e098b81/stdout:
hello
```

Some additional information on filters: You can filter on UUID (`uuid`), hostname (`hostname`), architecture (`arch`), OS (`os`), IP (`ip`, or `ip_range` for CIDR notation), MAC address (`mac`), and tags (`tags`).

You can also combine filters

```
# set a client filter on the 1.0.0.0/24 network for Windows clients
cc filter os=windows ip=1.0.0.0/24
```

To clear the current filter, you can use `clear cc filter`.

Another example for a basic Linux command would be to set a filter for Linux hosts and run something like `cc exec ls /` to get a directory listing of the client's root; we'll leave this as an exercise if you want to experiment more here.

Tags are set at VM boot with

```
vm config tag
```

The `miniccc` client itself can also add tags to the info for the VM the client is running on. This enables third-party tools to upstream information about a VM to minimega via `miniccc`. Tags are key/value pairs, and are added simply by using the `-tag` switch on a running `miniccc` instance:

```
./miniccc -tag foo bar
```

### Examining Responses

We previously saw that we can use `cc responses all` to print all responses received from `cc`. Let's now go into more detail on the `cc responses` API.

The `responses` command simply outputs all files of the specified command, indexed by ID as shown in `cc commands`, with the name of the file. If you don't want the filename shown, you can suffix the response command with `raw`:

```
# display our responses without the client and file header
cc responses all raw
```

Additionally, note that `all` is a keyword that otherwise replaces a command ID. We can view responses based on ID with

```
cc responses id
```

Keep in mind that if we start a new machine and runs `miniccc`, it will issue all past commands if it meets the prior filters.

Let's again use `cc commands` to list our past command ID numbers:

```
$ cc commands
host   | id | prefix | command             | responses | background | sent       | received | level | filter
ubuntu | 1  |        | [echo hello]        | 4         | false      | []         | []       |       |
ubuntu | 2  |        | [cmd /c echo hello] | 2         | false      | []         | []       |       | os=windows
```

You can delete commands you don't want to run on future `miniccc` sessions by referencing the ID number

```
$ cc delete command 1
```

or delete them all with

```
$ cc delete command all
```

You can also delete past responses from `<base>/miniccc_responses` with

```
$ cc delete responses 1
```

or delete them all with

```
$ cc delete responses all
```

### Command Prefixes

It's useful to group similar commands into named groups. The `cc prefix` command allows creating groups of commands by associating all issued commands after a `cc prefix` command with that prefix:

```
$ cc prefix holdmybeer
cc filter ip=100.0.0.0
cc exec echo 'this'
cc exec echo 'is'
cc exec echo 'a'
cc exec echo 'test'
clear cc prefix

cc commands
```

You can then use the prefix name instead of a command id when using `cc responses`:

```
cc responses holdmybeer raw
```

or when deleting commands:

```
cc commands delete holdmybeer
```

### Background Commands

Sometimes you want to execute a command on the client that doesn't return, such as a daemon or other agent. To tell `cc` not to wait on a response for a given command, use `cc background` instead of `cc exec`:

```
cc filter os=linux
cc background sleep 30
cc process list all
host   | name | uuid                                 | pid  | command
ubuntu | 3   | bc59214a-86aa-46cb-8810-a62dca455f9d  | 2306 | sleep 30
ubuntu | 3   | fa3ce3a5-27f0-4eac-8937-72fe1da13722  | 2319 | sleep 30
cc process kill 2306
```

This launched the `sleep 30` command in the background. When using background mode, `miniccc` will execute the command and immediately move on to the next command queued to run.

We also used the `cc process` API to `list all` backgrounded processes and then to `kill` a running background process by supplying a process ID (in this case, `2306`). Be careful with the filtering on process killing, so that you don't kill needed processes.

If the virtual machine reboots after connecting to `cc`, `miniccc` will prevent a reconnection from that UUID.

### File I/O

The `cc` API supports sending and receiving files to and from the client. Files can be specified by name or glob (wildcard). When sending files, they must be rooted in a specific directory provided by minimega.

#### Sending Files

In order to send files to a client, the files must be rooted in the `files` subdirectory in the minimega base directory. By default minimega uses `/tmp/minimega/files`. Let's send a simple bash script to the client by placing a file `foo.bash` in `/tmp/minimega/files` (or whatever base directory your minimega uses). Have `foo.bash` do something simple, like `echo`:

```
#!/bin/bash
echo "hello cc!"
```

Now we'll send it to the client by using the `cc send` command.

```
# send `foo.bash` to all clients
cc send foo.bash
cc commands
```

Clients will fetch the files from minimega before moving on to any other commands. You can inspect file `send` commands with `cc commands`, just like we did with `cc exec` before.

We can also send globs (wildcard) of files, using the `*` operator. For example:

```
minimega$ cc send somedirectory/*
```

Files will appear in the `files` subdirectory in the client's base directory. By default, this is `/tmp/miniccc/files`.

At this point we can both send and execute a file on the client. Let's execute the script we sent a moment ago:

```
# execute foo.bash
cc exec bash /tmp/miniccc/files/foo.bash
```

After the client responds to the `cc exec` command, we can check the output.

```
cc responses 3
```

#### Receiving Files

Receiving files is just like sending files, except that you can specify any path on the client to receive files from. Globs (wildcards) work with receiving files too, so you can receive entire directories of files.

Let's modify our example above and send a bash script that creates a file, execute it, and then get that file back. We'll have our bash script create the file `/foo/bar.out`, and name it `bar.bash`:

```
#!/bin/bash
mkdir /foo
echo "hello cc!" >> /foo/bar.out
```

```
cc send bar.bash
cc exec bash /tmp/miniccc/files/bar.bash
cc recv /foo/bar.out
```

Use `cc responses` to check the file received after it completes. You'll notice in the output that the file `bar.out` was received and stored in the response subdirectory `/foo`. This is because the `/foo` subdirectory was specified with the `recv` command.

```
# check on our received file
cc responses 6
```

### Full Filesystem Access

The `cc mount` API can be used to mount the guest filesystem on the host. This is accomplished by a 9p server integrated into `miniccc` that serves the guest's filesystem over the existing connection to the command and control server. `cc mount` is fully integrated with namespaces - you may mount a VM's filesystem to the head node, regardless of which host is actually running the VM. In order to mount the filesystem, you'll need to use either the name or UUID of the VM. For example, to mount the filesystem for our `u1604` VM to `/mnt`:

```
cc mount u1604 /mnt
```

To list existing mounts, run it with no arguments:

```
cc mount
```

Let's unmount the VM:

```
clear cc mount /mnt
```

Without an argument, `clear cc mount` clears all mounts; in general, a UUID or name or path can be supplied as an optional argument. This also occurs when you call `clear cc`.

### TCP Tunnels

The `cc` interface allows creating forward and reverse TCP tunnels over the `cc` connection, including over `virtio-serial` connections. This is similar to the forward and reverse tunnel support in `ssh`. To create a forward tunnel, that is, a listening port on the minimega host that is tunneled to a destination host and port from the perspective of the client, use `cc tunnel`. When creating a forward tunnel the UUID of the client **must** be specified. The destination host can be `localhost` or any other host reachable from the client.

Let's start a process listening on a port and create a tunnel

```
clear cc responses
cc filter uuid=bc59214a-86aa-46cb-8810-a62dca455f9d
cc background bash python -m SimpleHTTPServer
cc tunnel bc59214a-86aa-46cb-8810-a62dca455f9d 4444 localhost 8000
```

From the node running the VM navigate to http://127.0.0.1:4444 and it will redirect the traffic over the serial `cc` channel to the virtual machine.

You can also tunnel in the opposite direction. A reverse tunnel, a listening port on the client tunneled to a host and port reachable from the minimega host, can be created by using `cc rtunnel`. Reverse tunnels do not require a UUID to be specified, and instead use the current client filter to restrict which clients create the tunnel. That means you can tunnel a port for every client to a resource outside of the experiment.

Start a web server on the host and tunnel out from the virtual machine:

```
# python -m SimpleHTTPServer 8080
clear cc responses
clear cc filter
cc rtunnel 5555 localhost 8080
```

Now on the virtual machine if you connect to localhost:5555 you will see the web server from the host.

You can't delete tunnels without killing `miniccc` on the VM or the minimega process.

### Logging

You may adjust the log level of clients at runtime from minimega:

```
minimega$ cc log level debug
```

This will apply to clients matching the current filter.

### Authors

The minimega authors

Created: 30 May 2017

Last updated: 25 May 2022
