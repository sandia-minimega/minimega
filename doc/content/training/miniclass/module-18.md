# Module 18 - Scripting Commands

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-18-scripting-commands/).

## Introduction

You can script minimega commands using mm files, the `-e` flag, python bindings, and a command port exposed via a JSON-encoded Unix socket. Keep in mind certain commands will block for completion and others will not. You may need to wait for a command to fully complete before continuing with your script.

## MM Files

MM files, also known as read files, are simply commands listed line by line.

The `history` command will give you a print out of all the past executed commands that executed successfully. You can copy and paste these into a new file to generate a mm file.

```
minimega:/tmp/minimega/minimega$ history
ubuntu: vm config cdrom /home/ubuntu/tinycore.iso
vm config memory 128
vm config net 0
web root /home/ubuntu/minimega/misc/web 9001
vm launch kvm linux[1-5]
vm start all
```

minimega also has a `write` command that writes the command history to a file.

```
minimega:/tmp/minimega/minimega$ write /home/ubuntu/commands.mm
```

minimega can then be killed and started again, with the commands you want to be executed read in from a file

```
minimega:/tmp/minimega/minimega$ nuke
2016/08/30 04:00:17 FATAL client.go:90: server disconnected
[1]+  Done                    minimega/bin/minimega -nostdin
root@ubuntu:~# minimega/bin/minimega -nostdin &
[1] 6293
root@ubuntu:~# minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

root@ubuntu:~# minimega/bin/minimega -attach
CAUTION: calling 'quit' will cause the minimega daemon to exit
use 'disconnect' or ^d to exit just the minimega command line

minimega:/tmp/minimega/minimega$ read /home/ubuntu/commands.mm
2016/08/30 04:00:28 WARN vlans.go:334: Blacklisting manually specified VLAN 0
minimega:/tmp/minimega/minimega$ .columns name,memory,vnc_port vm info
host   | name   | memory | vnc_port
ubuntu | linux1 | 128    | 41812
ubuntu | linux2 | 128    | 44049
ubuntu | linux3 | 128    | 44307
ubuntu | linux4 | 128    | 41757
ubuntu | linux5 | 128    | 42555
```

## E Flag

minimega provides a `-e` flag to execute whatever command you want and return the result.

It is optional to string quote the command

```
root@ubuntu:~# minimega/bin/minimega -e ".columns name,memory,vnc_port vm info"
host   | name   | memory | vnc_port
ubuntu | linux1 | 128    | 41812
ubuntu | linux2 | 128    | 44049
ubuntu | linux3 | 128    | 44307
ubuntu | linux4 | 128    | 41757
ubuntu | linux5 | 128    | 42555
```

```
root@ubuntu:~# minimega/bin/minimega -e .columns name,memory,vnc_port vm info
host   | name   | memory | vnc_port
ubuntu | linux1 | 128    | 41812
ubuntu | linux2 | 128    | 44049
ubuntu | linux3 | 128    | 44307
ubuntu | linux4 | 128    | 41757
ubuntu | linux5 | 128    | 42555
```

Here is an example script that will launch every qcow in a directory as a new VM

```
#!/bin/bash

# path to disk images
IMAGES="/tmp/images"

# pointer to invoke a running minimega with the -e flag
MM="/usr/local/bin/minimega -e"

# launch VMs, all with 512MB of RAM and using one of each
# disk images in a directory
$MM vm config memory 512

for i in `ls $IMAGES`
do
    $MM vm config disk $IMAGES/$i
    $MM vm launch kvm vm-$i
done

$MM vm start all
```

## Command Port

The command port can also be interfaced by external programs. It is a UNIX domain socket in `<base>/minimega`. The command port uses JSON encoded commands and responds using the following schema:

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
