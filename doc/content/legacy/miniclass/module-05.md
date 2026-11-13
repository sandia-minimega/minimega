# Module 5 - Starting and Stopping Minimega

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-5-starting-and-stopping-minimega/).

## Introduction

minimega can now be started. KVM requires special permissions and minimega must be run as root unless permissions are modified.

## Launch Interactively

You can start minimega in your current session, but if the connection fails, minimega will close

```
# minimega
minimega$
```

## Launch Daemon

It is recommended to instead run minimega as a daemon

```
# minimega -nostdin &
```

There are multiple ways to interact with the daemon

## minimega Attach

You can attach to the daemon and execute multiple commands using minimega's attach flag

```
# minimega -attach
minimega$
```

## minimega -e

You can execute a single command with the -e flag

```
# minimega -e host name
foo
```

## Command Port

There is also a Unix domain socket that accepts JSON encoded commands located at <base>/minimega. The JSON encoded commands and responses use the following schema.

```
{
    "name": "Command",
    "properties": {
    "Original": {
        "type": "string",
        "description": "string form of command, with arguments separated by whitespace",
        "required": true
    },
    }
}

{
    "name": "localResponse",
    "properties": {
    "Resp": {
        "type": "array",
        "items": {
        "type": "Response"
        }
        "description": "array if responses to a single command"
    },
    "Rendered": {
        "type": "string",
        "description": "pre-rendered output of the Resp object according to output rendering rules"
    },
    "More": {
        "type": "bool",
        "description": "true if additional responses to the command are incoming"
    },
    }
}

{
    "name": "Response",
    "properties": {
    "Host": {
        "type": "string",
        "description": "host this response was created on"
    },
    "Response": {
        "type": "string",
        "description": "simple string response (exclusive to Header/Tabular)
    },
    "Header": {
        "type": "array",
        "items": {
        "type": "string"
        },
        "description": "column headers for tabular data"
    },
    "Tabular": {
        "type": "array",
        "items": {
        "type": "array",
        "items": {
            "type": "string"
        }
        },
        "description": "tabular data, each column is an array of strings"
    },
    "Error": {
        "type": "string",
        "description": "Error, if any"
    }
    }
}
```

## Bash Script

Here is a bash script that helps with various tasks such as logging as well as launching minimega. You may find it useful when launching minimega.

```
#!/bin/sh
#Run as root
#Delete old logs:
rm -f /var/log/mini.log

#SSD Maintenance:
#USE IF SSD
fstrim -v /

#Sync time.
ntpdate time.nist.gov;

#minimega starts its own dnsmasq process having the service running can cause issues
service dnsmasq stop;

#clean up potential things that were left by orphaned minimega instances:
rm -rf /tmp/mini.log
rm -rf /tmp/minimega/
mkdir -p /tmp/;
mkdir -p /tmp/files;

# Make sure certain kernel functions exist
# These two will fail if you attempt to put minimega in a VM without nesting support
modprobe kvm_intel;
modprobe kvm;

# Needed networking components
modprobe openvswitch;
modprobe 8021q;

# Define the max number of partitions so nbd-mount won't error
modprobe nbd max_part=8;

# Finally launch minimega in a tmux process that will keep running even if networking fails.
tmux kill-session -t minimega 2>/dev/null
tmux new -s minimega -d
tmux send-keys -t minimega "cd /home/ubuntu/minimega" C-m
tmux send-keys -t minimega "bin/minimega -degree 4 -nostdin -filepath="/data/mmfiles" -context ${1} -logfile="/var/log/mini.log" -console -v=false" C-m

# Start a web interface to interact with minimega on port 9001
tmux kill-session -t miniweb 2>/dev/null
tmux new -s miniweb -d
tmux send-keys -t miniweb "cd /home/ubuntu/minimega && sleep 5" C-m
tmux send-keys -t miniweb "bin/miniweb" C-m
```

Which you can then make executable with:

```
chmod +x launchme.sh
```

And run with:

```
./launchme.sh mynamespace
```

## Stopping minimega

There are a number of ways to stop minimega. Let's go over some of the ways.

### Gracefully

The most graceful way is to run the quit command twice

```
root@ubuntu:~# minimega -attach
CAUTION: calling 'quit' will cause the minimega daemon to exit
use 'disconnect' or ^d to exit just the minimega command line

minimega:/tmp/minimega/minimega$ quit
CAUTION: calling 'quit' will cause the minimega daemon to exit
If you really want to stop the minimega daemon, enter 'quit' again
minimega:/tmp/minimega/minimega$ quit
2016/08/25 14:07:04 FATAL client.go:90: server disconnected
```

### Nuke

Sometimes `quit` doesn't always work. The minimega api documents a `nuke` command.

After a crash, the VM state on the machine can be difficult to recover from.

Nuke attempts to kill all instances of QEMU, remove all taps and bridges, and removes the temporary minimega state on the harddisk. This should be run with caution.

```
root@ubuntu:~# minimega -nostdin &
[1] 31597
root@ubuntu:~# minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

root@ubuntu:~# minimega -attach
CAUTION: calling 'quit' will cause the minimega daemon to exit
use 'disconnect' or ^d to exit just the minimega command line

minimega:/tmp/minimega/minimega$ nuke
2016/08/25 14:11:08 ERROR container.go:1507: cgroup_root unmount: no such file or directory
2016/08/25 14:11:09 FATAL client.go:90: server disconnected
[1]+  Done                    minimega -nostdin
```

### Pkill

There may be times where there is no minimega console to attach to in those cases you can use `pkill`. Note this will not clean up networking taps, bridges, clean up state, or delete logs.

```
root@ubuntu:~# minimega -nostdin &
[1] 31617
root@ubuntu:~# minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

root@ubuntu:~# ps aux | grep mini
root     31617  0.4  0.9 229824 19640 pts/1    Sl   14:13   0:00 minimega -nostdin
root     31630  0.0  0.0  14224  1088 pts/1    S+   14:13   0:00 grep --color=auto mini
root@ubuntu:~# pkill mini
[1]+  Done                    minimega -nostdin
root@ubuntu:~# pkill qemu
root@ubuntu:~# ps aux | grep mini
root     31636  0.0  0.0  14224  1084 pts/1    S+   14:13   0:00 grep --color=auto mini
```

### Reboot

Use caution before doing a reboot.

If minimega reconfigured networking upon reboot there is a chance dhcp won't work, and you will have to manually connect with a keyboard and mouse to fix networking.

Make sure PXE boot is not enabled in the bios by default, or you could delete everything upon reboot.

Depending on your hardware a reboot can take 30s-15min

With that said a reboot usually fixes a number of things.

```
reboot
```
