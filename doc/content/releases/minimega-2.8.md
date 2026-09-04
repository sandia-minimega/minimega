# minimega 2.8 release notes

## Introduction

The minimega team is pleased to announce the release of minimega 2.8. This release includes several new features and bug fixes as well as updates to support the latest dependency packages. Golang minimum requirements were raised to golang 1.17 to address security concerns. This version of minimega brings Command and Control upgrades, Networking upgrades, Mesh upgrades, and much more.

## Major changes and new features

#### [miniccc]: add option for only executing a command once (#1508)

This allows, for example, the `shutdown -r now` command to be sent to a VM using command and control without putting the VM into a perpetual reboot loop. This is a contrived example, since a minimega VM can be rebooted without using command and control, but it still gets the point across. There may be other commands that are sent via command and control that end up causing a VM to reboot that would benefit from this as well.

```
New cc commands added:
  cc exec-once command ...
  cc background-once command ...
```

#### [minimega] updates to qemu arguments to support newer versions (#1506)

#### [minimega] Adding config option for switching from xHCI to EHCI for USB controller.

This allows the use of usb3.0 devices in VMs. This is the new default

#### [minimega] add -recovery flag to recover VMs after minimega crash, assumes VMS are still running

NOTE: this only works for KVM VMs for now (not containers)

If the `-recovery` flag is provided, the `-force` flag is not (the `-force` flag will take precedence), and the minimega unix socket still exists (e.g., after minimega crashing for some reason), minimega will attempt to recover any QEMU KVM VMs still running that are mapped to\
valid minimega namespaces and VMs per the myriad of files that minimega writes to `/tmp/minimega`.

When minimega creates a QEMU KVM VM, it sets the `-name` option to minimega's VM ID and sets the `-uuid` option to the UUID generated or specified for the minimega VM. These two `qemu-system` process flags, along with the myriad of files present in `/tmp/minimega` are used to piece together which VMs minimega started, in what namespaces, with what taps an VLANs, etc. minimega then uses this information to (re)populate its internal data structures such that commands like `vm info` show the correct data. It also uses this information to reconnect VM VNC and CC connections.

#### [minimega] Add "vm net bond" API command (#1486)

Bond two or more network interfaces together for a VM. All interfaces being bonded must be on the same bridge or the command will fail. If all the interfaces being bonded are not on the same VLAN, the bond uses the VLAN from the first interface. If at least one of the bonded interfaces is configured for QinQ, the bond will be configured for QinQ as well.

#### [minimega] Add "qinq" option to "vm config net" API command (#1487)

Sets the VLAN mode to dot1q-tunnel for the network tap in Open vSwitch, using the VLAN specified in the net spec as the outer VLAN for QinQ. This commit also adds the "qinq" column to the output of the "vm info" API command.

#### [minimega] Use channel instead of mutex to serialize cli and meshage commands (#1474)

There are cases where one mesh node sends commands to another, and while waiting for the other node to respond it sends the original node a command as well and waits for it to respond, leading to a blocking race condition. Below is an example:

```
When adding a network interface to an existing VM using "vm net add", the node the "vm net add" command is executed on sends a command over the mesh to the node the VM is running on and waits for a response.

       vm net add
head --------------> compute

When the compute node adds the network interface to the VM, it checks to
see if the VLAN alias for the interface exists. If it doesn't, it
creates the Alias-to-ID mapping and publishes it out to all the nodes in
the mesh and waits for a response.

        vlans add
head <-------------- compute

This is where the blocking race condition occurs. The head node cannot
process the "vlan add" command from the compute node until the compute
node responds to the "vm net add" command, but the compute node is
waiting for the head node to respond to the "vlans add" command before
it responds to the "vm net add" command.

      vlans add resp
head -------X-------> compute

      vm net add resp
head <------X--------- compute

The reason the head node cannot respond to the "vlan add" command is due
to the cmdLock mutex (which isn't protecting data but is instead
ensuring commands are processed in a serial fashion), needed to process
the "vlan add" command, being held by the function that made the "vm net
add" call.

The fix for this was to switch from using a mutex ensuring commands are
run in serial to using a channel, because the channel can queue commands
to prevent blocking.
```

#### [minimega] Add flag for sending logs to a node via the mesh

The new flag is `-lognode` and takes the name of a node on the mesh. When minimega is deployed to other nodes using `deploy launch` the flag will automatically be included using the hostname the deployment is being run from.

#### [miniccc] Support TCP and UDP connectivity testing from miniccc (#1457)

minimega, miniccc, and ron were updated to support executing tcp/udp connectivity tests directly from miniccc agents. ron was updated to include a new `ConnTest` command struct that encapsulates the endpoint to test against, how long to wait, and what UDP packet to send (if necessary).

miniccc was updated to include a handler for the new `ConnTest` command that simply tries to dial the endpoint (in the case of TCP), and if necessary write the UDP packet to the socket (in the case of UDP). minimega was updated to include support for a new "cc test-conn" command, as well as adding a "connectivity" column to the "cc commands" table. Documentation for minimega's "cc" command was also updated to include details and examples of how to use the new "test-conn" command.

"cc test-conn" allows users to test network connectivity from a guest to the given IP or domain name and port. The wait timeout should be specified as a Go duration string (e.g. 5s, 1m). If "udp" is used, a "base64 udp packet" that will generate a valid response must be specified. Results of the test will be written to the command's STDOUT file, whether it passed or failed. An example test is as follows: cc test-conn tcp 10.0.0.68 443 wait 10s

#### [minirouter] Add "router fw" commands to minirouter to add iptables FORWARD rules

Both minimega and minirouter were updated to support configuring iptables in the minirouter instance using new "router fw" commands. minirouter was updated to include support for "fw" commands for both global iptables rules and grouped rules using iptables chains. minimega was updated to include support for "router fw" commands that generate the minirouter "fw" commands in the config script sent to the minirouter instance via miniccc. Documentation for minimega's "router" command was also updated to include details and examples of how to use the new "fw" commands.

## Minor changes and new features

#### Change minimum Golang Version go 1.17; update dependencies (#1507) updates gopacket, dns, crypto, and net dependencies

#### [miniccc] Track and record exit code for exec'ed commands (#1479)

#### [minimega] Don't shadow dst variable when looping through inject files (#1478)

#### [minimega] add CLI flag for setting default VLAN range for namespaces (#1489)

This is useful for avoiding VLAN ID ranges that are used for production or lab networks that minimega may be tied into for hardware-in-the-loop access to real devices/workstations/servers. The default for the flag is the same as the current default range, so not setting the flag provides backwards compatibility.

#### [minimega] Retry (with backoff) finding disk partitions for injects (#1490)

#### [minimega] Improve mesh file transfer decision process using hashes (#1477)

Don't watch transfer_ directories to avoid unnecessary hashing, added logic for which file to get when not in -headnode mode, Added CLI flag for enabling file hashing, Added -headnode and -hashfiles flags and additional code to support modes. Include files in miniccc_responses directories when hashing files

#### [minimega] Periodically publish status of file transfers (#1476)

A new "status update" construct was added to provide long running commands the ability to periodically send status updates about the long running command to one or more nodes in the mesh.

A "status update" API command was added to allow users to adjust how\
often, if at all, status updates are sent. A developer article was added to help developers understand how the construct was added and how it can be used. While testing status updates, it was discovered that a lot of error logs were being generated for things that weren't actually errors, so that was fixed as well.

#### [minimega] show mod time in file listing and add recursive flag (#1469)

#### [miniccc] Improve reconnect logic for serial clients

The miniccc client and ron server have been updated to better support reconnection capabilities when using the virtual serial port in QEMU virtual machines. Past merge 850c4450 added support for reconnecting over serial after a virtual machine restart, but didn't address connection issues that arise after a VM has been paused or restored from snapshot. When a VM is paused, the server side of the serial connection eventually disconnects and resets. When the VM is resumed, the client is still connected to the virtual serial port in the VM but messages are no longer making it to the server because of the server-side reset.

Since the virtual serial port in the client never changed, the client never sees an EOF and is still able to write to the port without error. The same thing as above happens when a VM is restored from snapshot... the server side makes a new connection to the unix socket that's mapped to the VM's virtual serial port, and the client is still connected to the virtual serial port in the VM like it was prior to the snapshot. In order to allow for the client to detect the disconnect, a HEARTBEAT message type was added and the server was updated to send a HEARTBEAT message to the client every so often (default is 5s).

The client does nothing with this message, but can expect to receive it consistently, and can now timeout and reset if no messages are received within a certain amount of time (default is 13s). The Linux miniccc client is able to reset by simply closing its connection to the virtual serial port and reconnecting. This approach fails on Windows, however, and the only way to reconnect to the virtual serial port on Windows is to restart the miniccc client process. The easiest way to do this is to run the miniccc client process as a Windows service that's configured to restart on failure, and exit the process when the client detects the need to reset the connection.

To support this, the Windows version of the miniccc client has been updated to include a `-install` flag that can be used to install it as a Windows service that will restart on failure.

#### [miniccc] Support miniccc reconnection over serial port

React to async QMP virtual serial port change events, which are triggered when the miniccc client connects to the virtual serial port in a VM. When a connection is opened client-side, the server connects to the virtual serial port and waits for the initial handshake message. Client-side, miniccc has been updated to wait after connecting to the virtual serial port to give the server time to react to the QMP event and make it's connection to the serial port first.

## Bug fixes

#### [minimega] Cancel serial connection retries if miniccc agent is too old (#1482)

Continually reconnecting to an older version of miniccc that doesn't maintain a connection (mainly an issue in Windows VMs) prevents mounting the VMs file system using `cc mount` from working properly and in some cases will lock up the entire minimega process completely.

#### [minimega] Log error instead of returning if cc mount path can't be unmounted (#1481)

When unmounting a VM's file system using `clear cc mount`, don't immediately return on errors, but instead log the error and allow the remaining commands to still attempt to be run.

#### [Docker] Fix duplicate MM_RECOVER env variable in Docker start script (#1503)

#### [pyapigen] Allow zeros in minimega command args (#1491)

The Python command `' '.join([str(v) for v in cmd if v])` will exclude\
command args that are `0`, which breaks, for example, capturing traffic\
on interface 0 of a VM. Instead, use test `if v is not None`. This will allow empty strings to\
get through, but that shouldn't cause a problem.

#### [iomeshage] Short-circuit file get process if there's only one mesh node (#1498)

Right now, using `file get` against a directory fails if there's only one node in the mesh. In general, if only one node exists, then obviously all files will be present on the node, so the `file get` process can be skipped.

#### [miniweb ]Update noVNC to 1.3.0; fix VNC Framebuffer Recording (#1488)

Update novnc directory to version 1.3.0 document modifications to noVNC source so future updates are easier. Changed noVNC to set minimega params in url hash cleaner option than injecting our params into input fields after load. Disabled support for extended key event messages in noVNC minimega vnc recorder does not support these messages currently.\
Improved comments for noVNC to make future upgrades easier .Fixed recording vnc framebuffers change rfbplay to not error if no offset provided by http form

#### [miniweb] Prevent Locking when Downloading File in miniweb (#1475)

Changed logic so that downloading files in miniweb is non blocking, make miniweb use new connection per download, and make minimega unlock cmdLock early when streaming file

#### [minimega] Get files to be sent to miniccc via mesh before sending (#1472)

When a file is sent to a VM using cc, it's expected that the file is already on the node running the VM. This commit adds support for automatically getting the file to be sent to a VM from other nodes in the mesh, similar to how it's done for VM disk images when they are\
launched.

#### [minimega] Prevent dnsmasq from leaking DNS results

This commit updates the command line flags for dnsmasq to explicitly point to a non-existent resolv.conf file rather than letting it use the default /etc/resolv.conf. This prevents leakage of DNS results as described in #1421. This commit also adds an option to the `dns configure` command for adding upstream DNS servers. Adding an upstream server simply adds a\
nameserver entry to the (initially non-existent) resolve.conf file dnsmasq is configured to use. Upstream servers added will immediately be recognized by dnsmasq since it polls the file for changes.

#### [minimega] Only clear local mounts for 'clear cc' (no subcommand). Fixes #1463. (#1466) Fixes issue where clear cc filter clears mounts

#### [miniccc] version the cc message so heartbeats work across revisions (#1470)

This is an attempt to make miniccc somewhat future-proof. Right now, if minimega is updated then miniccc running in existing images will continually fail and reconnect because currently we only start the heartbeat on the server if the commit versions between the server and miniccc match. This is somewhat okay, except for when cc mount needs to be used. By versioning the cc message, we can now accurately determine if older miniccc agents can be used with newer versions of deployed servers.

#### [minimega] Allow user to specify which broadcast IP to use for meshage (#1458)

In some cases, cluster compute nodes have multiple interfaces and networks they can communicate with each other over, and one may be desirable over another for meshage comms. By default, minimega uses `255.255.255.255` as the broadcast address, and when multiple interfaces are present, the one acting as the default route is the one used. This commit adds a new command line option, `-broadcast`, for users to specify which broadcast IP to use for the mesh. It defaults to `255.255.255.255`, so this commit should be backwards compatible.

#### [miniccc] fix map race conditions in ron server (#1461)

In commit dd04c33f introduced a bug in the ron server code that handles serial cc connections failing to protect maps accessed concurrently with their associated mutexes that would randomly manifest itself into race conditions.

#### [minimega] Fix for ns snapshot when using a mesh.

#### [minimega] Add vm snapshot command.

Supports creating a migrate and disk file for a single vm. Upgrade ns snapshot command to also create disk file.
