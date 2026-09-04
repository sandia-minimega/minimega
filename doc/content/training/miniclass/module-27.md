# Module 27 - Custom Containers

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-27-custom-containers/).

## Introduction

We used a prebuilt filesystem in a prior module. Let's make custom filesystems and boot them.

WARNING Here be Dragons: When you build container filesystems with Docker and LXC you are diving into a realm of things I wouldn't expect to be supported.

You may find better luck running Docker and LXC containers in a nested manner in a KVM.

## Adding static binaries to containerfs

```
cd ~
wget https://storage.googleapis.com/minimega-files/minimega-2.2-containerfs.tar.bz2
tar xf minimega-2.2-containerfs.tar.bz2
```

Create a go web server:

```
package main

import (
    "fmt"
    "net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hello World!")
}

func main() {
    http.HandleFunc("/", handler)
    fmt.Println("Server running...")
    http.ListenAndServe(":8080", nil)
}
```

Compile the go web server to a binary

```
GOOS=linux GOARCH=386 go build ./webserver.go
```

Copy this to containerfs:

```
cp webserver /home/ubuntu/containerfs/
```

Edit `/home/ubuntu/containerfs/init`

```
nano /home/ubuntu/containerfs/init
```

Before bash run the webserver binary

```
/webserver &
```

### Booting the container filesystem

```
minimega -attach
$ vm config filesystem /home/ubuntu/containerfs/
$ vm config net 0
$ vm config snapshot true
$ vm launch container custom-cfs
$ vm start custom-cfs
```

There should be a web server running on port 8080 on the container.

## Building a custom containerfs with shared libraries

The minirouter container has a clean build script you can tweak for your application.

[github.com/sandia-minimega/minimega/blob/master/misc/uminirouter/build.bash](https://github.com/sandia-minimega/minimega/blob/master/misc/uminirouter/build.bash)

In summary:

- A basic file structure is made
- A 64-bit binary of busybox is downloaded.
- Symbolic links for busy box compiled programs are created
- The `PATH` variable is updated
- Init and Preinit are copied
- Binaries are copied from the host
- The program `ldd` is used to locate shared libraries and they are copied over
- Scripts get tweaked to use `/bin/sh`
- A root user is created with no password
- And a package is made.

## LXC

```
apt-get install lxc
lxc-create -t ubuntu -n ubuntu
```

This downloads and generates a root file system in `/var/lib/lxc/ubuntu/rootfs/` of Ubuntu 16.04

Before we can boot this we need to add an `init`

```
cat > /var/lib/lxc/ubuntu/rootfs/init << EOF
#!/bin/sh

ifconfig lo up
ifconfig veth0 up
dhclient -v veth0

/usr/sbin/sshd &

bash
EOF
```

Now we need to fix the permissions on `init`

```
chmod 755 /var/lib/lxc/ubuntu/rootfs/init
```

`sshd` needs a folder that doesn't already exist, let's fix that:

```
cd /var/lib/lxc/ubuntu/rootfs/
mkdir -p var/run/sshd
chmod 0755 var/run/sshd
```

### Booting the container filesystem

```
minimega -attach
$ vm config filesystem /var/lib/lxc/ubuntu/rootfs/
$ vm config net 0
$ vm config snapshot false
$ vm launch container ubuntu1604
$ vm start ubuntu1604
```

The `mega_bridge` adapter is configured in a bridged adapter manner which you can do following the later networking modules.

Any changes you make will be permanent, including applications you download from `apt-get`.

In order to get `apt-get` working you will need to fix the `PATH` variable with:

```
export PATH=$PATH:/usr/local/sbin/
export PATH=$PATH:/usr/sbin/
export PATH=$PATH:/sbin
export PATH=$PATH:/usr/local/bin/
export PATH=$PATH:/usr/bin/
export PATH=$PATH:/bin
```

I also needed to set a nameserver with:

```
echo "nameserver 8.8.8.8" > resolv.conf
```

From there `apt-get` worked and I was able to install packages like `python` and `nano`.

I typed exit and changed to `snapshot true` and booted 50 of these containers using <440mB RAM total

## Docker

Install Docker

[www.digitalocean.com/community/tutorials/how-to-install-and-use-docker-on-ubuntu-16-04](https://www.digitalocean.com/community/tutorials/how-to-install-and-use-docker-on-ubuntu-16-04)

Download the Docker Ubuntu image.

```
docker pull ubuntu
```

And run the image

```
docker run -it ubuntu /bin/bash
```

From the Docker container's `/bin/bash`

```
apt-get update
apt-get install isc-dhcp-client net-tools iputils-ping python
```

Open another terminal:

```
docker ps
docker export e50ef793a69d -o ubuntu.tar.gz
docker kill e50ef793a69d
```

Export the filesystem

```
mkdir dubuntu
cd dubuntu
tar xf ../ubuntu.tar.gz
```

Create an `init` file

```
cat > init << EOF
#!/bin/sh

ifconfig lo up
ifconfig veth0 up
dhclient -v veth0

bash
EOF
```

Now we need to fix the permissions on `init`

```
chmod 755 /var/lib/lxc/ubuntu/rootfs/init
```

### Booting the container filesystem

```
minimega -attach
$ vm config filesystem /home/ubuntu/dubuntu/
$ vm config net 0
$ vm config snapshot true
$ vm launch container dubuntu1604
$ vm start dubuntu1604
```

## Special Notes

There is no `/dev/stdout` or `/dev/stderr` so any image using those will need to be modified.

Depending on your base image, it may be difficult to install other packages, like `dhclient`.

Containers are still pretty experimental.
