# minimega Docker

## Install Docker

Follow the official installation instructions: [Install Docker Engine](https://docs.docker.com/engine/install/)

For development purposes, it maybe helpful to add your user to the `docker` group: `sudo usermod -aG docker $USER`


## Build the minimega Docker image

> NOTE: Currently, only minimega, miniweb, miniccc, minirouter, and protonuke
> will exist in the minimega docker image. If you need additional binaries, add
> them to the Dockerfile using the `COPY --from=gobuilder …` directive.

> NOTE: The docker image needs to be built from the base directory of the
> minimega repository.

```bash
docker build -t minimega -f docker/Dockerfile .

# Ensure the build was successful
docker run -it minimega /opt/minimega/bin/minimega --version
```

## Start the minimega Docker container

> NOTE: The additional privileges and system mounts (e.g. /dev) are required for
> the openvswitch process to run inside the container and to allow minimega to
> perform file injections.

> NOTE: If the `deploy launch` minimega command is used to initialize a
> multi-node minimega cluster, then a directory containing SSH keys will likely
> need to be mounted as a volume as well (and can be read-only). An example
> would be `-v /root/.ssh:/root/.ssh:ro`.

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

The container runs the `start-minimega.sh` script as PID 1, which takes care of starting openvswitch, miniweb, and finally minimega. This means the minimega logs will be available in the container logs via Docker (`docker logs minimega`).


# Using Docker Compose

If you followed the [Docker installation instructions](https://docs.docker.com/engine/install/), then `docker compose` should already be installed. Verify this by running `docker compose version`.  If it's not, then install it: `sudo apt install docker-compose-plugin`

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

On Ubuntu, `~/.bash_aliases` should be auto-sourced by `~/.profile` or `~/.bashrc` on login, so the source command is only needed to load them into current session.

## minimega and miniweb configuration

By default, the following values are set for minimega:

```shell
MM_BASE=/tmp/minimega
MM_FILEPATH=/tmp/minimega/files
MM_BROADCAST=255.255.255.255
MM_PORT=9000
MM_DEGREE=2
MM_CONTEXT=minimega
MM_LOGLEVEL=info
MM_LOGFILE=/var/log/minimega.log
MM_FORCE=true
MM_RECOVER=false
MM_CGROUP=/sys/fs/cgroup
```

By default, the following values are set for miniweb:

```shell
MINIWEB_ROOT=/opt/minimega/misc/web
MINIWEB_HOST=0.0.0.0
MINIWEB_PORT=9001
```

These values can be overwritten either by passing environment variables to
Docker when starting the container or by binding a file to
`/etc/default/minimega` in the container that contains updated values.

> NOTE: If a value is specified both as an environment variable to Docker and in
> the file bound to `/etc/default/minimega`, the value in
> `/etc/default/minimega` will be used.

> NOTE: If the port is changed for minimega or miniweb and standard container
> networking is used (not host networking), then the `ports` section in your
> `docker-compose.yml` or `-p` arguments to `docker run` will need to be updated
> to the new value(s) specified.

Additional values can be appended to the minimega command by using:

```shell
MM_APPEND="-hashfiles -headnode=foo1"
```
