# miniweb

miniweb is the web interface for a running minimega. It lists VMs, hosts,
VLANs, and files, opens VNC, terminal, and Android consoles in the browser,
draws the network graph, and can expose a minimega console and a JSON command
API. This page covers starting miniweb, what each URL does, how namespaces map
to URLs, and how to add authentication and TLS.

You need a running minimega first; see [Running minimega](running.md). The
Docker image starts miniweb for you; see [Running in Docker](docker.md).

## How it connects

miniweb talks to minimega over the command socket in minimega's base
directory, `<base>/minimega`, so it has to run on a host that runs minimega,
normally the head node. Every command it sends is prefixed with
`.record false`, so browsing does not show up in minimega's `history`. If the
socket goes away, miniweb redials on the next request.

## Starting miniweb

The packages install the binary as `/opt/minimega/bin/miniweb` (linked to
`/usr/bin/miniweb`) and the web assets under `/opt/minimega/web`. In a source
checkout the assets are the repository's `web/` directory, which is also the
default `-root`, so either run miniweb from the repository root or pass
`-root`:

```bash
$ miniweb -root /opt/minimega/web
```

Then open <http://localhost:9001/>.

| Flag | Default | Purpose |
|---|---|---|
| `-addr` | `:9001` | Listen address as `host:port`. |
| `-base` | `/tmp/minimega` | minimega base directory; must match the daemon's `-base`. |
| `-root` | `web` | Directory holding the templates and static files. |
| `-namespace` | | Restrict miniweb to one namespace (see [Namespaces](#namespaces)). |
| `-passwords` | | JSON password file for HTTP basic authentication. |
| `-bootstrap` | `false` | Interactively create the `-passwords` file, then exit. |
| `-cert`, `-key` | | PEM certificate and key. Both are required to serve HTTPS. |
| `-console` | | Path to the minimega binary. Enables `/console`, `/command`, and `/commands`. |
| `-level`, `-logfile`, `-v` | `error`, none, `true` | Logging. |

In the Docker image, `start-minimega.sh` starts miniweb before minimega with
`-root=$MINIWEB_ROOT -addr=$MINIWEB_HOST:$MINIWEB_PORT`; the defaults are
`/opt/minimega/web`, `0.0.0.0`, and `9001`.

### Static directories and the bundled documentation

At startup miniweb registers every subdirectory of `-root` as a static file
server at `/<name>/`: `/css/`, `/js/`, `/novnc/`, `/xterm.js/`, and so on. The
Docker image copies the built documentation to `/opt/minimega/web/docs`, which
is why <http://localhost:9001/docs/> works inside a container. You can publish
your own files the same way: create a directory under `-root` and it appears
at `/<name>/`.

## Pages

| Path | What it shows |
|---|---|
| `/` | Redirects to `/vms`. |
| `/vms` | VM table from `vm info`; VMs in the `quit` or `error` state are hidden. `/vms#top` switches to the `vm top` view. |
| `/vms/new` | Form that launches one VM: a name, the kind (`kvm` or `container`), and one of the saved configurations that `vm config restore` lists. |
| `/tilevnc` | Screenshot grid of every VM; click a tile to connect. |
| `/hosts` | Output of `host`. |
| `/vlans` | VLAN aliases from `vlans`. |
| `/graph` | Interactive graph of VMs and the VLANs that join them. |
| `/files/` | Listing from `ns run file list`; append a subdirectory, for example `/files/images/`. Requesting a single file downloads it through `file stream`. There is no upload. |
| `/namespaces` | Every namespace, with links to its VM and VLAN pages. Answers 501 when `-namespace` is set. |
| `/minibuilder` | Diagram editor for sketching topologies; `/minibuilder/save` returns the drawing as an XML download. |
| `/console` | Web terminal attached to minimega. Requires `-console`. |
| `/command`, `/commands` | JSON command API. Require `-console`. |

The tables are fed by JSON endpoints you can also call from scripts:
`/vms/info.json`, `/vms/top.json`, `/hosts.json`, `/vlans.json`,
`/namespaces.json`, and `/files.json?path=<dir>`. Each returns a list of
objects, one per row, keyed by column name plus `host`.

### The VM table

The State column carries the controls: a play icon for VMs in the `BUILDING`
or `PAUSED` state (`vm start`), a pause icon for `RUNNING` VMs (`vm stop`),
and an X that runs `vm kill`. Killed VMs drop out of the table because `quit`
VMs are filtered; they still exist until you `vm flush` from the CLI. The VNC
column links to `/vm/<name>/connect`. Several columns (uptime, type, disks,
IPv6, taps, tags, active CC) are hidden by default.

### Per-VM routes

| Path | Method | Purpose |
|---|---|---|
| `/vm/<name>/connect/` | GET | Console page: noVNC for KVM VMs, xterm.js for containers, the Android console for Android VMs. |
| `/vm/<name>/connect/ws` | WebSocket | Raw tunnel to the VM's `vnc_port` (KVM) or `console_port` (container) as reported by `vm info`. Not used by Android VMs. |
| `/vm/<name>/screenshot.png` | GET | Current screenshot. `?size=<pixels>` scales it; `&base64=1` returns a `data:` URI instead of PNG bytes. Containers answer 404. |
| `/vm/<name>/start`, `/vm/<name>/stop`, `/vm/<name>/kill` | POST | Run `vm start`, `vm stop`, or `vm kill` for that VM and return the rendered response. |
| `/vm/<name>/android/display/ws` | WebSocket | Live PNG frames from the Android emulator. |
| `/vm/<name>/android/api/v1/emulator/status` | GET | Emulator status. |
| `/vm/<name>/android/api/v1/emulator/gps`, `.../rotation` | POST | Set the GPS fix or the device orientation. |

## Connecting to VMs

For a KVM VM the connect page loads noVNC and opens the WebSocket; miniweb
dials the VM's host and VNC port directly and copies bytes in both directions.
Containers get an xterm.js terminal on the container's console port. minimega
keeps a small scrollback buffer for each container console and lets several
viewers share it, so a newly opened terminal shows recent output instead of a
blank screen. Android VMs get a console that streams emulator frames over
`/vm/<name>/android/display/ws` and drives input, GPS, and orientation through
the emulator; see [Android VMs](android.md).

Because miniweb connects to the host and port that `vm info` reports, the
miniweb host must be able to reach the VNC and console ports on every node in
the cluster. If the miniweb log says it cannot connect, check the firewall
between the nodes.

## Namespaces

miniweb selects a namespace in one of two ways.

With `-namespace <name>`, every command runs in that namespace, the URL prefix
described next is disabled, and `/namespaces` answers 501.

Without it, prefix any path with a namespace: `/foo/vms` shows the VMs of
namespace `foo`, `/foo/vlans` its VLAN aliases, `/foo/files/` its file
listing, and `/foo/vm/<name>/connect` a console. Unprefixed paths use whatever
namespace the head node is currently in. `/namespaces` lists the namespaces
with links to each, and a bare `/foo` redirects to `/foo/`.

## Console and command API

Pass `-console` with the path to the minimega binary to enable three routes:

```bash
$ miniweb -root /opt/minimega/web -console /usr/bin/minimega
```

`/console` opens a terminal in the browser. Each visit spawns
`<path> -attach` in a pseudo-terminal. miniweb passes no `-base`, so the
attached minimega takes its base directory from `MM_BASE` in miniweb's
environment, failing that from `/etc/minimega/minimega.conf`, and otherwise
uses the default; export `MM_BASE` when your daemon uses a `-base` that is
not recorded there.

`/command` accepts a `POST` with one JSON object and returns the responses as
JSON. `columns` and `filters` are optional and become `.columns` and `.filter`
prefixes on the command:

```bash
$ curl http://localhost:9001/command -d '{"command": "vm info", "columns": ["name", "state"], "filters": ["state=running"]}'
```

`/commands` accepts a `POST` with a JSON array of such objects and runs them in
order.

The namespace for `/command` and `/commands` comes from the URL prefix or
`-namespace`, as for every other page. Without `-console`, all three routes
answer 501.

!!! warning
    These routes run any minimega command, including `shell`. Enable them
    only behind authentication and TLS, or bind `-addr` to an address that
    untrusted networks cannot reach. See
    [Security considerations](security.md).

## Authentication

miniweb supports per-path HTTP basic authentication, which lets you limit a
user to a namespace or to a single VM. Create the password file interactively
with `-bootstrap`:

```text
$ miniweb -bootstrap -passwords minimega.passwd
Configure /
Username: jon
Password:
Confirm Password:

Add additional users (Ctrl-D when finished):
Path: /vm/fritz
Username: fritz
Password:
Confirm Password:

Add additional users (Ctrl-D when finished):
Path:
```

The result is a JSON list of entries with bcrypt-hashed passwords:

```json
[
	{
		"path": "/",
		"username": "jon",
		"password": "JDJhJDEwJFZzSjNIRjFVbjc5Z2ZReUpxVmRZMS5WU2thaUNHS3RIczk5eExFMS5kdy5VUEQuY1pub1FT"
	},
	{
		"path": "/vm/fritz",
		"username": "fritz",
		"password": "JDJhJDEwJGc0VC93YkNtWVZjMWlteXF1dldrRy5zRkZWVm5GSzMxSjdNZFgwdGp6eklnZmtVaHVuUmhh"
	}
]
```

Start miniweb with the file, dropping `-bootstrap`:

```bash
$ miniweb -passwords minimega.passwd
```

A request must satisfy one of the entries whose `path` is a prefix of the
request path; paths that match no entry need no credentials at all. Because
matching is by prefix, with the file above both jon (`/`) and fritz
(`/vm/fritz`) can open `/vm/fritz`, while only jon can open `/vms`. To limit a
user to namespace `foo`, use the path `/foo/`. The format is deliberately
simple so that a script can generate it when you have many rules.

## TLS

Basic authentication sends credentials in the clear, so serve HTTPS whenever
you use it. miniweb takes a PEM certificate and key and refuses to start if
only one of them is given. To generate a self-signed pair:

```bash
$ openssl req -x509 -newkey rsa:4096 -nodes -keyout key.pem -out cert.pem -days 365
$ miniweb -cert cert.pem -key key.pem -passwords minimega.passwd
```

## See also

- [Running minimega](running.md) and [Running in Docker](docker.md)
- [VNC](vnc.md) and [Android VMs](android.md)
- [Namespaces](namespaces.md)
- [Command line and scripting](cli.md) for the command socket that miniweb uses
- [Security considerations](security.md)
- [Tools overview](../tools.md)
