# minimega docker

### Install docker

```bash
sudo apt-get install docker.io
```

### Build the minimega docker image

> NOTE: Currently, only minimega, miniweb, miniccc, minirouter, and protonuke
> will exist in the minimega docker image. If you need additional binaries, add
> them to the Dockerfile using the `COPY --from=gobuilder …` directive.

> NOTE: The docker image needs to be built from the base directory of the
> minimega repository.

```bash
docker build -t minimega -f docker/Dockerfile .
```

### Start the minimega docker container

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

The container runs the `start-minimega.sh` script as PID 1, which takes care of
starting openvswitch, miniweb, and finally minimega. This means the minimega
logs will be available in the container logs via Docker.

---

# Using docker-compose

### Install docker-compose

```bash
VERSION=`git ls-remote https://github.com/docker/compose | grep refs/tags | grep -oP "[0-9]+\.[0-9][0-9]+\.[0-9]+$" | sort | tail -n 1`
sudo curl -ksL "https://github.com/docker/compose/releases/download/${VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

### Start the minimega docker container

```bash
docker-compose up -d
```

---

# Extras

### Convenience aliases

```bash
cat <<EOF >> ~/.bash_aliases
alias minimega='docker exec -it minimega minimega '
alias ovs-vsctl='docker exec -it minimega ovs-vsctl'
EOF
source ~/.bash_aliases
```

### minimega and miniweb configuration

By default, the following values are set for minimega:

```
MM_BASE=/tmp/minimega
MM_FILEPATH=/tmp/minimega/files
MM_BROADCAST=255.255.255.255
MM_PORT=9000
MM_DEGREE=2
MM_CONTEXT=minimega
MM_LOGLEVEL=info
MM_LOGFILE=/var/log/minimega.log
```

By default, the following values are set for miniweb:

```
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
