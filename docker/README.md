# minimega Docker

## Install Docker

Follow the official installation instructions:
[Install Docker Engine](https://docs.docker.com/engine/install/)

> [!TIP]
> For development purposes, it may be helpful to add your user to the `docker`
> group: `sudo usermod -aG docker $USER`

## Start the minimega Docker container

> [!NOTE]
> The additional privileges and system mounts (e.g. /dev) are required for the
> openvswitch process to run inside the container and to allow minimega to
> perform file injections.

> [!WARNING]
> If the `deploy launch` minimega command is used to initialize a multi-node
> minimega cluster, then a directory containing SSH keys will likely need to be
> mounted as a volume as well (and can be read-only). An example would be
> `-v /root/.ssh:/root/.ssh:ro`.

```bash
docker run -d \
  --name minimega \
  --hostname minimega \
  --privileged \
  -p 9000:9000/udp \
  -p 9001:9001 \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /var/log/minimega:/var/log/minimega \
  -v /tmp/minimega:/tmp/minimega \
  -e MM_LOGFILE=/var/log/minimega/minimega.log \
  --health-cmd "mm version" \
  ghcr.io/sandia-minimega/minimega:latest
```

This command uses the image of the latest release. To use an image that you
built in the [section below](#build-the-minimega-docker-image), replace the
last line with `minimega`.

The container runs the `start-minimega.sh` script as PID 1, which takes care of
starting openvswitch, miniweb, and finally minimega. On container stop, it
forwards the shutdown signal to minimega and removes stale PID/socket state.
This means the minimega logs will be available in the container logs via Docker
(`docker logs minimega`).

The image includes the documentation in `/opt/minimega/docs`. With the default
`MINIWEB_ROOT`, miniweb also serves it at
[http://localhost:9001/docs/](http://localhost:9001/docs/).

## Using Docker Compose

If you followed the
[Docker installation instructions](https://docs.docker.com/engine/install/),
then `docker compose` should already be installed. Verify this by running
`docker compose version`. If it's not, then install it using the command:
`sudo apt install docker-compose-plugin`

Start the minimega Docker container with Docker Compose:

```bash
cd docker/
docker compose up -d
docker compose logs -f  # CTRL+C to stop following logs
```

The Compose file uses the image of the latest release,
`ghcr.io/sandia-minimega/minimega:latest`. You do not have to build an image.
To get a newer release later, run `docker compose pull` and then
`docker compose up -d` again.

To use a different image, set the `MINIMEGA_IMAGE` environment variable. For
example, to use the latest build of the `master` branch:

```bash
MINIMEGA_IMAGE=ghcr.io/sandia-minimega/minimega:master docker compose up -d
```

To use the image that you built in the [section below](#build-the-minimega-docker-image):

```bash
MINIMEGA_IMAGE=minimega docker compose up -d
```

The Compose file uses the network of the host (`network_mode: host`) and
publishes no ports. minimega and miniweb listen on the host, and the bridges
and taps of minimega are on the host. The container has the host name of the
machine, so each node of a cluster has a different name.

The Compose file sets `MM_LOGFILE`, so the minimega log is in
`/var/log/minimega/minimega.log` on the host.

## Build the minimega Docker image

> [!IMPORTANT]
> The docker image needs to be built from the base directory of the minimega
> repository.

```bash
docker build -t minimega -f docker/Dockerfile .

# Ensure the build was successful
docker run --rm -it minimega /opt/minimega/bin/minimega -version
```

## Extras

### Host wrapper script and tab completion

The `docker/minimega-wrapper.sh` script lets you run `minimega` commands from
the host as if the binary were installed locally, by transparently forwarding
them to `docker exec` inside the running `minimega` container. It also takes
care of only allocating a pseudo-TTY (`-it`) when stdout is actually a
terminal, so its output can be safely piped or redirected (e.g. for bash tab
completion, see below) without picking up spurious `\r` characters.

Install it as `/usr/local/bin/minimega` so it's available on your `$PATH`:

```bash
sudo install -m 0755 docker/minimega-wrapper.sh /usr/local/bin/minimega
```

With that in place, you can run minimega commands directly:

```bash
minimega -e vm info
minimega -attach
```

#### Installing tab completion

minimega can generate its own tab-completion script for `bash`, `zsh`, or
`fish` via the `-completion` flag. Since the wrapper script forwards
`-completion` through to the minimega binary inside the container, you can
use it the same way you would the native binary.

To enable it for the current shell session:

```bash
source <(minimega -completion bash)  # bash
source <(minimega -completion zsh)   # zsh
minimega -completion fish | source   # fish
```

To enable it for every session, write the generated script somewhere your
shell's completion system will find it, e.g.:

```bash
# bash
minimega -completion bash | sudo tee /usr/share/bash-completion/completions/minimega >/dev/null

# zsh (write to a directory in your fpath)
minimega -completion zsh | sudo tee /usr/share/zsh/site-functions/_minimega >/dev/null

# fish
minimega -completion fish | sudo tee /usr/share/fish/vendor_completions.d/minimega.fish >/dev/null
```

With completion installed, typing `minimega -e vm sta<TAB>` will complete to
`minimega -e vm start`, the same way tab completion works at the minimega
command prompt itself.

> [!TIP]
> The `minimega` Docker image ships with the `bash-completion` package and a
> pre-generated completion script at
> `/usr/share/bash-completion/completions/minimega`, so bash tab completion
> works out of the box for interactive shells inside the container, e.g.
> `docker exec -it minimega bash`.

### Convenience aliases

```bash
cat <<EOF >> ~/.bash_aliases
alias mm='docker exec -it minimega minimega -e'
alias mminfo='mm .columns name,state,ip,snapshot,cc_active vm info'
alias mmsum='mm .columns name,state,cc_active,uuid vm info summary'
alias minimega='docker exec -it minimega minimega'
alias ovs-vsctl='docker exec -it minimega ovs-vsctl'
EOF

source ~/.bash_aliases
```

> [!TIP]
> On Ubuntu, `~/.bash_aliases` should be auto-sourced by `~/.profile` or
> `~/.bashrc` on login, so the source command is only needed to load them into
> the current session.

### minimega and miniweb configuration

The order of precedence for configuration options is:
1. Existing environment variables
2. Variables in `/etc/default/minimega`
3. A set of defaults in the `start-minimega.sh` script

The defaults set for minimega are:

```shell
MM_BASE=/tmp/minimega
MM_FILEPATH=/tmp/minimega/files
MM_BROADCAST=255.255.255.255
MM_VLANRANGE=101-4096
MM_PORT=9000
MM_DEGREE=1
MM_CONTEXT=minimega
MM_LOGLEVEL=info
MM_LOGFILE=/var/log/minimega.log
MM_FORCE=true
MM_RECOVER=false
MM_CGROUP=/sys/fs/cgroup
MM_ABSSNAPSHOT=false
```

The defaults set for miniweb are:

```shell
MINIWEB_ROOT=/opt/minimega/web
MINIWEB_HOST=0.0.0.0
MINIWEB_PORT=9001
MINIWEB_APPEND=
```

The default miniweb root contains the bundled documentation under `docs/`.
When `MINIWEB_ROOT` points elsewhere, the documentation remains available
inside the container at `/opt/minimega/docs` but is not served automatically.

These values can be overwritten either by passing environment variables to
Docker when starting the container or by binding a file to
`/etc/default/minimega` in the container that contains updated values.

> [!NOTE]
> If a value is specified both as an environment variable to Docker and in
> the file bound to `/etc/default/minimega`, the existing non-empty environment
> variable will be used.

> [!WARNING]
> If the port is changed for minimega or miniweb and standard container
> networking is used (not host networking), then the `ports` section in your
> `docker-compose.yml` or `-p` arguments to `docker run` will need to be updated
> to the new value(s) specified.

Additional values can be appended to the minimega command by using the
`MM_APPEND` environment variable, for example:

```shell
MM_APPEND="-hashfiles -headnode=foo1"
```

In the same way, the `MINIWEB_APPEND` environment variable appends values to
the miniweb command. Images of release 3.4.0 and earlier ignore this variable.
For example, to require a password and to use TLS, with the three files
mounted into the container:

```shell
MINIWEB_APPEND="-passwords=/etc/miniweb/passwords -cert=/etc/miniweb/cert.pem -key=/etc/miniweb/key.pem"
```

### Open vSwitch configuration

The docker container `start-minimega.sh` script takes care of starting
Open vSwitch server using the
[`ovs-ctl`](https://docs.openvswitch.org/en/latest/ref/ovs-ctl.8/) program.

Additional values can be appended to the `ovs-ctl start` command by using the
`OVS_APPEND` environment variable, for example if you are running the
ovsdb-server externally and only need the Open vSwitch client:

```shell
OVS_APPEND="--no-ovsdb-server --no-ovs-vswitchd"
```

The client then needs the database socket of the external server. Mount the
Open vSwitch run directory of the host, usually `/var/run/openvswitch`, at the
same path in the container.

The script has the ability to optionally add host Ethernet interface(s) to a
Open vSwitch bridge using the `OVS_HOST_IFACE` environment variable. The format
of the variable is `<bridge>:<port>[,<port>,...]`.

> [!CAUTION]
> The OVS bridge to add the interface(s) to __*must be specified*__

For example, the following will add the `eth0` host interface to the `phenix`
OVS bridge, creating the bridge if it doesn't exist already:

```shell
OVS_HOST_IFACE: phenix:eth0
```

Multiple interfaces can also be specified:

```shell
OVS_HOST_IFACE=phenix:eth0,eth1,eth2
```
