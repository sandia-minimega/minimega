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
starting openvswitch, miniweb (unless `MINIWEB_ENABLED=false`), and finally
minimega. On container stop, it forwards the shutdown signal to minimega and
removes stale PID/socket state.
This means the minimega logs will be available in the container logs via Docker
(`docker logs minimega`).

The image includes the documentation in `/opt/minimega/docs`. With the default
`MINIWEB_ROOT`, miniweb also serves it at
[http://localhost:9001/docs/](http://localhost:9001/docs/).

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

# Firecracker VMs

The image includes Firecracker and the jailer v1.17.0, and the tools that
Firecracker VMs need at run time (`acl`, `dmsetup`, `iproute2`, and
`qemu-utils`). The build needs network access to `github.com` to get
Firecracker. It checks the SHA-256 of the release tarball against the
`FIRECRACKER_SHA256` build argument, or against the `.sha256.txt` file of the
release when `FIRECRACKER_SHA256` is empty.

To install a different version, set `FIRECRACKER_VERSION`, and set
`FIRECRACKER_SHA256` to its SHA-256 or to an empty value. To build the image
without Firecracker, set `FIRECRACKER_VERSION` to an empty value:

```bash
docker build --build-arg FIRECRACKER_VERSION= -t minimega -f docker/Dockerfile .
```

## Start the container for many Firecracker VMs

```bash
docker run -d \
  --name minimega \
  --hostname minimega \
  --privileged \
  --cap-add ALL \
  --network host \
  --ulimit nofile=1048576:1048576 \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /var/log/minimega:/var/log/minimega \
  -v /tmp/minimega:/tmp/minimega \
  -v /var/lib/minimega:/var/lib/minimega \
  -e MM_FILEPATH=/var/lib/minimega/files \
  --health-cmd "mm version" \
  minimega
```

- Use one bind mount for the images (`MM_FILEPATH`) and for the Firecracker
  directory (`MM_FIRECRACKER_DIR`, default `/var/lib/minimega/firecracker`).
  minimega puts hard links to the disk images into each jail. A hard link
  cannot go across two mounts. The bind mount also keeps the VM disk writes
  out of the overlay2 storage driver.
- Set the open file limit. minimega uses several file descriptors for each
  Firecracker VM, so 10,000 VMs need more than 50,000. Docker Engine 29 sets
  a limit of 1024 in containers. `start-minimega.sh` sets the soft limit to
  the hard limit and shows a warning when the result is less than 100000.
- Do not use `--pids-limit`. Each VM adds processes and threads.
- Do not use a `--memory` value that is less than the total memory of all the
  VMs.

The `docker-compose.yml` file sets the open file limit and the bind mount for
`/var/lib/minimega/firecracker`. It does not use host networking.

## cgroup v2

On cgroup v2, a cgroup that holds processes cannot enable controllers for its
child cgroups. Before minimega starts, `start-minimega.sh` moves all the
container processes into the child cgroup `/sys/fs/cgroup/init`. Then it
enables the `cpu`, `memory`, and `pids` controllers in
`/sys/fs/cgroup/cgroup.subtree_control`. The parent cgroup of the jails can
then use these controllers. The script does not change the host root cgroup.

## OOM killer

`start-minimega.sh` sets `oom_score_adj` to -1000 for the Open vSwitch
daemons. Set `MM_OOM_SCORE_ADJ=-1000` to also protect minimega.

> [!CAUTION]
> The VMs that minimega starts get the `oom_score_adj` value of minimega. With
> -1000, the OOM killer cannot stop the VMs either.

## Docker daemon

1. Add `"live-restore": true` to `/etc/docker/daemon.json`. The containers
   then continue to run when the Docker daemon restarts.

   ```json
   {
     "live-restore": true
   }
   ```

2. Reload the Docker daemon: `sudo systemctl reload docker`.
3. Stop automatic upgrades of the Docker packages. An upgrade can restart the
   Docker daemon and containerd at a time that you do not control. Upgrade
   the packages manually instead.

   ```bash
   sudo apt-mark hold docker-ce docker-ce-cli containerd.io  # Docker packages
   sudo apt-mark hold docker.io containerd                   # Ubuntu packages
   ```

## Host settings

minimega shows a `firecracker host tuning` warning at startup for each of these
settings that is not correct.

- Mount cgroup v2 with `favordynmods`. Without it, each cgroup change of the
  jailer is slow when many VMs run. Do this again after each boot:
  `sudo mount -o remount,favordynmods /sys/fs/cgroup`
- Set transparent hugepages to `madvise`:
  `echo madvise | sudo tee /sys/kernel/mm/transparent_hugepage/enabled`
- Stop KSM: `echo 0 | sudo tee /sys/kernel/mm/ksm/run`
- Stop systemd-udevd from setting a MAC address on each new minimega
  interface. Write this to `/etc/systemd/network/10-minimega.link`, then run
  `sudo udevadm control --reload`:

  ```ini
  [Match]
  OriginalName=mega_tap*

  [Link]
  MACAddressPolicy=none
  ```

- When Open vSwitch runs on the host, set `OOMScoreAdjust=-1000` in a
  systemd drop-in file for `ovs-vswitchd.service` and `ovsdb-server.service`.

# Extras

## Host wrapper script and tab completion

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

### Installing tab completion

minimega can generate its own tab-completion script for `bash`, `zsh`, or
`fish` via the `-completion` flag. Since the wrapper script forwards
`-completion` through to the minimega binary inside the container, you can
use it the same way you would the native binary.

To enable it for the current shell session:

```bash
source <(minimega -completion bash)   # bash
source <(minimega -completion zsh)    # zsh
minimega -completion fish | source    # fish
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
MM_FIRECRACKER_DIR=/var/lib/minimega/firecracker
MM_FIRECRACKER_UID_BASE=2000000000
MM_FIRECRACKER_LAUNCH_PARALLEL=0
MM_FIRECRACKER_START_PARALLEL=0
MM_OOM_SCORE_ADJ=
```

`MM_OOM_SCORE_ADJ` is empty by default, so the script does not change the
`oom_score_adj` of minimega. See [OOM killer](#oom-killer).

The defaults set for miniweb are:

```shell
MINIWEB_ROOT=/opt/minimega/web
MINIWEB_HOST=0.0.0.0
MINIWEB_PORT=9001
MINIWEB_ENABLED=true
```

Set `MINIWEB_ENABLED=false` to not start miniweb. With a value other than
`true` or `false`, the script stops before it starts any service.

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

## Open vSwitch configuration

The docker container `start-minimega.sh` script takes care of starting
Open vSwitch server using the
[`ovs-ctl`](https://docs.openvswitch.org/en/latest/ref/ovs-ctl.8/) program.

Additional values can be appended to the `ovs-ctl start` command by using the
`OVS_APPEND` environment variable, for example if you are running the
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
