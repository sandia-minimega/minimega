# minimega docker

### Install docker

  ```bash
  $ sudo apt-get install docker.io
  ```

### Build the minimega docker image

> NOTE: Currently, only minimega, miniweb, miniccc, and protonuke will exist in the minimega docker image. If you need additional binaries, add them to the Dockerfile using the `COPY --from=gobuilder …` directive.

> NOTE: The docker image needs to be built from the base directory of the minimega repository.

  ```bash
  $ docker build -t minimega -f docker/Dockerfile .
  ```

### Start the minimega docker container

> NOTE: The additional privileges and system mounts (e.g. /dev) are required for the openvswitch process to run inside the container and to allow minimega to perform file injections.

```bash
docker run -d -it \
  --name minimega \
  --hostname minimega \
  --privileged \
  --cap-add ALL \
  -p 9000:9000/udp \
  -p 9001:9001 \
  -v /tmp/minimega:/tmp/minimega \
  -v /var/log/minimega:/var/log/minimega \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
  --health-cmd "minimega -e version" \
  minimega
```

The container runs systemd as PID 1, which takes care of starting openvswitch, minimega, and minimweb.

---

#  Using docker-compose

### Install docker-compose

```bash
$ VERSION=`git ls-remote https://github.com/docker/compose | grep refs/tags | grep -oP "[0-9]+\.[0-9][0-9]+\.[0-9]+$" | sort | tail -n 1`
$ sudo curl -ksL "https://github.com/docker/compose/releases/download/${VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
$ sudo chmod +x /usr/local/bin/docker-compose
```

### Start the minimega docker container

```bash
$ docker-compose up -d
```

---

# Extras

### Convenience aliases

```bash
$ cat <<EOF >> ~/.bash_aliases
alias minimega='docker exec -it minimega minimega '
alias ovs-vsctl='docker exec -it minimega ovs-vsctl'
EOF
$ source ~/.bash_aliases
```

### Starting miniweb

miniweb gets started in the container automatically.

