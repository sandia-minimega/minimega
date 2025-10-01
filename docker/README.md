# minimega Docker

## Install Docker

Follow the official installation instructions:
[Install Docker Engine](https://docs.docker.com/engine/install/)

> [!TIP]
> For development purposes, it maybe helpful to add your user to the `docker`
> group: `sudo usermod -aG docker $USER`

## Build the minimega Docker image

> [!IMPORTANT]
> The docker image needs to be built from the base directory of the minimega
> repository.

```bash
docker build -t minimega -f docker/Dockerfile .

# Ensure the build was successful
docker run -it minimega /opt/minimega/bin/minimega --version
```

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
  --cap-add ALL \
  -p 9000:9000/udp \
  -p 9001:9001 \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /var/log/minimega:/var/log/minimega \
  -v /tmp/minimega:/tmp/minimega \
  --health-cmd "mm version" \
  minimega
```

The container runs the `start-minimega.sh` script as PID 1, which takes care of
starting openvswitch, miniweb, and finally minimega. This means the minimega
logs will be available in the container logs via Docker (`docker logs minimega`).

# Using Docker Compose

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

# Extras

## Convenience aliases

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

## minimega and miniweb configuration

The order of precedence for configuration options is:
1. Existing environment variables
2. Variables in `/etc/default/minimega`
3. A set of defaults in the `start-minimega.sh` script

The defaults set for minimega are:

```shell
MM_BASE=/tmp/minimega
MM_FILEPATH=/tmp/minimega/files
MM_BROADCAST=255.255.255.255
MM_PORT=9000
MM_DEGREE=1
MM_CONTEXT=minimega
MM_LOGLEVEL=info
MM_LOGFILE=/var/log/minimega.log
MM_FORCE=true
MM_RECOVER=false
MM_CGROUP=/sys/fs/cgroup
```

The defaults set for miniweb are:

```shell
MINIWEB_ROOT=/opt/minimega/web
MINIWEB_HOST=0.0.0.0
MINIWEB_PORT=9001
```

These values can be overwritten either by passing environment variables to
Docker when starting the container or by binding a file to
`/etc/default/minimega` in the container that contains updated values.

> [!NOTE]
> If a value is specified both as an environment variable to Docker and in
> the file bound to `/etc/default/minimega`, the value in
> `/etc/default/minimega` will be used.

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

## Open vSwitch configuration

The docker container `start-minimega.sh` script takes care of starting
Open vSwitch server using the
[`ovs-ctl`](https://docs.openvswitch.org/en/latest/ref/ovs-ctl.8/) program.

Additional values can be appended to the `ovs-ctl start` command by using the
`OVS_APPEND` environment variable, for example if you are runnng the
ovsdb-server externally and only need the Open vSwitch client:

```shell
OVS_APPEND: --no-ovsdb-server --no-ovs-vswitchd
```

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
