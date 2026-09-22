# minimega configuration reference

Detailed configuration behavior for the native binary, the Docker image, and
the systemd unit. Paths are relative to the minimega repository root. Keep every
launcher aligned with the flags registered in `cmd/minimega/main.go` and
`pkg/minilog/minilog.go`; the compiled `minimega -h` output is authoritative for
flag names and defaults.

## Native process

The native binary does not load YAML, JSON, or TOML configuration files, and
does not treat `.env` as a dotenv file. Local client modes have one exception:
they read `MM_BASE` from `/etc/minimega/minimega.conf` when needed to find the
daemon socket.

Startup values come from compiled defaults, supported `MM_*` process environment
variables, and command-line flags:

| Purpose | Flags |
|---|---|
| Runtime state | `-base`, `-filepath`, `-cgroup` |
| Mesh and VLANs | `-context`, `-degree`, `-msa`, `-broadcast`, `-port`, `-vlanrange`, `-headnode` |
| Lifecycle | `-nostdin`, `-force`, `-recover`, `-panic`, `-hashfiles`, `-abssnapshot` |
| Logging | `-level`, `-logfile`, `-v`, `-verbose` |
| Alternate and client modes | `-e`, `-attach`, `-namespace`, `-pipe`, `-version`, `-cli`, `-completion`, `-suggest` |

### Flag syntax and interactions

Use Go flag syntax: `-name=value` works for every flag, `-name value` works for
non-boolean flags, and booleans use `-flag` or `-flag=false`. Parsing stops at
the first non-flag argument or `--`; when a flag is repeated, the last parsed
value wins. Put `-base` and `-namespace` before the command following `-e`.
`-namespace` affects only `-e` and `-attach`.

When `-recover=true`, minimega attempts recovery after mesh initialization even
if `-force=true`; `-force` only takes precedence when removing an existing
command socket. The compiled `-h` text for `-recover` still says "only if
-force is not set"; the implementation in `cmd/minimega/main.go` runs recovery
whenever `-recover` is set, so treat that help string as stale on this point.

When `-base` changes while `-filepath` retains its default value, minimega
rebases the file path to `<base>/files`.

### Startup command files

There is no automatic startup command file. Despite the `[file]...` text in the
startup usage line, normal server startup ignores positional arguments. Apply
runtime configuration explicitly after startup:

```bash
minimega -e read /path/to/experiment.mm
minimega -e read /path/to/experiment.mm check
```

`read <file>` executes commands in order, stops at invalid syntax, does not stop
when a valid command returns an error, and rejects nested `read` commands. The
`check` form validates syntax without executing commands. Lines starting with
`#` are comments. See `../examples/minimal-kvm.mm` in this skill directory for
a complete file.

### Environment-backed flag defaults

The process recognizes these environment variables as flag defaults:
`MM_BASE`, `MM_DEGREE`, `MM_MSA`, `MM_BROADCAST`, `MM_VLANRANGE`, `MM_PORT`,
`MM_FORCE`, `MM_RECOVER`, `MM_DAEMON`, `MM_CONTEXT`, `MM_FILEPATH`,
`MM_LOGLEVEL`, `MM_LOGFILE`, `MM_CGROUP`, `MM_PANIC`, and `MM_ABSSNAPSHOT`.
The mapping lives in `cmd/minimega/environment.go`. Explicit command-line flags
take precedence. They apply to server mode and to the `-e`, `-attach`, and
`-pipe` client modes alike. For those client modes, when neither `MM_BASE` nor
`-base` is set, minimega also reads `MM_BASE` from `/etc/minimega/minimega.conf`;
other configuration-file values are not read in client modes.

The process explicitly honors `GOMAXPROCS`; inherited environment variables are
also expanded from `$NAME` or `${NAME}` in runtime command string and list
arguments. The `.env` mentioned in command help is a runtime minicli command,
not a file; minimega does not discover or read dotenv files. Use `.env` to
inspect or change the daemon's environment. An empty value unsets a variable,
and changes last only for that process lifetime. `PATH` and tool-specific
variables still affect discovery and execution of external helpers.

## Effective precedence

- **Native:** explicit CLI flags, then supported process environment variables,
  then the compiled default. Repeated CLI flags use the last value.
- **Runtime scripts:** commands execute sequentially, so later commands can
  replace earlier runtime state.
- **Docker:** for generated flags, a non-empty container environment value wins
  over `/etc/default/minimega`, which wins over the wrapper default. A duplicate
  flag in `MM_APPEND` wins because it is appended last; explicit flags then win
  over native defaults.
- **systemd:** systemd resolves `EnvironmentFile=` and substitutes those values
  into explicit `ExecStart` flags; those flags win over native defaults.

## Docker container

minimega and miniweb are separate images built from separate targets in
`docker/Dockerfile`: `--target minimega` and `--target miniweb`. A build with
no `--target` produces the minimega image. `docker/README.md` covers
deployment.

The minimega image runs `docker/start-minimega.sh` as its default command. The
script starts Open vSwitch, waits for it, then execs minimega with `-nostdin`
and generated flags, so minimega runs as PID 1 and handles SIGTERM itself.

The miniweb image runs `docker/start-miniweb.sh`, which execs miniweb as PID 1.
miniweb needs the minimega socket directory (`/tmp/minimega` by default) and
the minimega network namespace, because it proxies VNC and container consoles
to per-VM shim ports. Compose uses `network_mode: "service:minimega"`;
`docker run` uses `--network container:minimega`. Consequently the miniweb
container publishes no ports, and `MINIWEB_PORT` is published by the minimega
container instead. Each minimega container start creates a new network
namespace, so restart miniweb after minimega restarts. The Compose
`depends_on` entry does this for Compose restarts, but not for crash restarts.

- `MM_BASE`, `MM_FILEPATH`, `MM_BROADCAST`, `MM_VLANRANGE`, `MM_PORT`,
  `MM_DEGREE`, `MM_CONTEXT`, `MM_LOGLEVEL`, `MM_LOGFILE`, `MM_FORCE`,
  `MM_RECOVER`, `MM_CGROUP`, and `MM_ABSSNAPSHOT` map to same-purpose minimega
  flags.
- `MINIWEB_ROOT`, `MINIWEB_HOST`, `MINIWEB_PORT`, `MINIWEB_BASE`, and
  `MINIWEB_LOGLEVEL` configure miniweb, and have defaults. `MINIWEB_LOGFILE`,
  `MINIWEB_NAMESPACE`, `MINIWEB_PASSWORDS`, `MINIWEB_CERT`, `MINIWEB_KEY`, and
  `MINIWEB_CONSOLE` have none; each maps to the same-named miniweb flag and is
  only passed when non-empty. `MINIWEB_CONSOLE=/console-attach.sh` enables the
  web console, which is an unauthenticated minimega command line unless
  `MINIWEB_PASSWORDS` is also set. `MINIWEB_APPEND` adds raw miniweb flags. `MINIWEB_BASE` must match
  the minimega container's `MM_BASE`.
- The Compose `miniweb` service is hardened for the reference deployment:
  `cap_drop: [ALL]`, `no-new-privileges`, `read_only: true`, and a tmpfs on
  `/tmp`. With `read_only`, `MINIWEB_LOGFILE` must point inside a mounted
  volume or miniweb exits with `read-only file system`. Removing the hardening
  is supported, and expected when debugging inside the container.
- `OVS_APPEND` adds raw `ovs-ctl start` arguments. `OVS_HOST_IFACE` uses
  `<bridge>:<port>[,<port>...]`.
- `MM_APPEND` adds otherwise unsupported minimega flags, such as
  `-msa=20 -hashfiles`.

Both startup scripts source `docker/load-defaults.sh`, so this precedence is
identical in both containers. Set values with Docker `-e`, Compose
`environment` or `env_file`, or a file mounted at `/etc/default/minimega`.
Existing **non-empty** container environment values take precedence; an unset
or empty value permits the file value, then the script default. The file parser
accepts simple `KEY=value` lines, strips only surrounding double quotes, and
does not source shell expressions.
This container-only file is distinct from the host service's
`/etc/minimega/minimega.conf`.

The wrapper expands scalar configuration values without shell quoting, so paths
and other single values cannot safely contain whitespace, glob characters, or
shell syntax. `MM_APPEND`, `MINIWEB_APPEND`, and `OVS_APPEND` are intentionally
split on whitespace: spaces separate shell-safe argument tokens but cannot be
preserved inside one argument. In `MM_APPEND`, a positional token stops parsing
later tokens. Because
it appears last, avoid accidentally duplicating generated flags unless an
override is intentional.

Docker defaults intentionally differ from native defaults, including
`MM_DEGREE=1`, `MM_LOGLEVEL=info`, `MM_LOGFILE=/var/log/minimega.log`, and
`MM_FORCE=true`. Additional container gotchas:

- Changing `MM_PORT` or `MINIWEB_PORT` also requires matching published ports,
  both of which belong to the minimega container.
- The `mm` wrapper (`docker/mm`) and the default Compose health check
  (`mm version`) run `minimega -e`, which honors `MM_BASE` from the container
  environment. A base set through Docker `-e` or Compose `environment` therefore
  reaches them automatically. A base set only in `/etc/default/minimega` is read
  by the startup script but not by `docker exec`, so pass `-base=<path>` to `mm`
  and the health check in that case. Update the `/tmp/minimega` volume mount in
  `docker/docker-compose.yml` when the host needs the relocated socket or files.
- Supplying a command after the image name replaces the Dockerfile `CMD` and
  skips that image's startup wrapper.
- The minimega image still ships the `miniweb` binary in `/opt/minimega/bin`,
  but does not start it and does not carry the web assets or documentation;
  those are in the miniweb image only.

## systemd service

The packaged unit at `misc/daemon/minimega.service` reads
`/etc/minimega/minimega.conf` through `EnvironmentFile=` and maps its `MM_*`
values to explicit startup flags; `MM_APPEND` is not supported. Keep the file to
literal `KEY=value` assignments and check `readlink -f` before editing because a
package may link it into `/opt/minimega`.

Restart the service after changing the environment file. After changing the unit
or a drop-in, run `systemctl daemon-reload` before restarting; replacing
`ExecStart` requires first clearing it with an empty `ExecStart=`. Because the
unit uses `Restart=on-success`, use `systemctl stop minimega` rather than runtime
`quit` when the service must remain stopped.

## Getting help

```bash
minimega -h                  # startup flags and compiled defaults
minimega -e help             # runtime command summary
minimega -e help vm config   # exact runtime command forms
minimega -cli                # machine-readable runtime CLI as JSON, then exit
minimega -completion bash    # generate bash, zsh, or fish completion
```

At an interactive or attached prompt, use `help` and `help <command>`. Pass the
correct `-base=<path>` before `-e` when the daemon uses a non-default base.
`-suggest` is an internal completion-script interface, not normal operator help.
