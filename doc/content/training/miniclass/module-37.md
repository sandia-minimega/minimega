# Module 37 - VNCDrone

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-37-vncdrone/).

## Introduction

In the bin directory you will find a binary for VNCDrone. This program automatically plays vnc files. Provide a directory with with [diskname].kb files that match existing disk names and it will play them all back.

## Help

```
root@ubuntu:~/minimega/bin# ./vncdrone -help
Usage of ./vncdrone:
  -base string
        minimega base directory (default "/tmp/minimega")
  -level string
        log level: [debug, info, warn, error, fatal] (default "debug")
  -logfile string
        log to file
  -nodes string
        node(s) running VMs
  -recordings string
        directory containing recordings
  -v    log on stderr (default true)
```

## Example

```
vncdrone -recordings /home/ubuntu/kb/ -nodes m[2-5]
```
