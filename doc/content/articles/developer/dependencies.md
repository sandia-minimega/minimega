# Dependencies

minimega is a single Go module, `github.com/sandia-minimega/minimega/v2`, that
requires Go 1.24 or newer. Dependencies are declared with Go modules and
vendored into the repository, so every build on every machine compiles the same
source. Vendoring also lets us carry local fixes without waiting for them to be
resolved upstream, and it keeps a license record for everything we ship.

## How dependencies are tracked

| File or directory | What it holds |
|---|---|
| `go.mod` | The module path, the Go version, the direct requirements, the indirect requirements (marked `// indirect`), and the `replace` directives for local forks. |
| `go.sum` | Checksums for every module version the build may use. |
| `vendor/` | A copy of the source of every package the build imports. |
| `vendor/modules.txt` | The version of each vendored module, which packages were taken from it, and any `replace` that produced it. Go verifies this against `go.mod` on every build. |
| `packages/` | Local forks of dependencies, wired in with `replace`. See [Local forks](#local-forks). |
| `LICENSES/` | One entry per third-party component. See [Licenses](#licenses). |

## Building against the vendor directory

Every `go` command must use the vendor directory rather than the module cache.
Set `GOFLAGS`:

```bash
export GOFLAGS=-mod=vendor
```

`scripts/env.bash` exports it along with `GOBIN`, `scripts/docs.bash` defaults
to it, and the CI workflows set it in the environment. If a build fails with
`inconsistent vendoring`, `go.mod` and `vendor/modules.txt` have drifted apart;
re-run `go mod vendor`.

## Adding a dependency

Before adding one, consider:

- Do we actually **need** it? For small amounts of functionality we prefer to
  reimplement rather than take on a dependency.
- What does it pull in with it? An indirect requirement is still a dependency
  we ship and have to keep current.
- Is the license compatible? minimega is GPLv3; see
  [Copyright](../contributing.md#copyright).

Ask the team if you are unsure -- it never hurts to get a second (or third)
opinion.

To add one:

```bash
go get example.com/some/module@v1.2.3
go mod tidy
go mod vendor
```

Prefer a tagged release over a pseudo-version on an untagged commit. Then add
the license to `LICENSES/`, run the tests, and commit `go.mod`, `go.sum`, and
the whole `vendor/` change together, as one commit.

## Updating a dependency

```bash
go get example.com/some/module@v1.3.0
go mod tidy
go mod vendor
```

Use `go list -m -u all` to see what has newer versions available. Update one
module per commit where you can, so a bad update is easy to revert. If the
module is one of the local forks below, port the fork's changes to the new
code first.

## Local forks

Never edit anything under `vendor/` by hand: the next `go mod vendor`
overwrites it. When a dependency needs a change that is not resolved upstream,
the change lives in `packages/<import path>/` and a `replace` directive in
`go.mod` points the import at it. `go mod vendor` then copies the local fork
into `vendor/`, so the import path in the code never changes.

| Import path | Replaced by |
|---|---|
| `github.com/dutchcoders/goftp` | `./packages/github.com/dutchcoders/goftp` |
| `github.com/goftp/server` | `./packages/github.com/goftp/server` |
| `github.com/stargrave/goircd` | `./packages/github.com/stargrave/goircd` |
| `github.com/thoj/go-ircevent` | `./packages/github.com/thoj/go-ircevent` |
| `github.com/Harvey-OS/ninep` | `github.com/jcrussell/ninep`, a fork hosted on GitHub rather than a local copy |

To change a fork, edit the files under `packages/`, run `go mod vendor`, and
commit the `packages/` and `vendor/` changes together so the two stay aligned.

## Licenses

`LICENSES/` holds one file per third-party component, named
`LICENSE.<short name>`. Where the component ships its license in the tree, the
entry is a symbolic link to it -- `LICENSE.dns` points at
`../vendor/github.com/miekg/dns/LICENSE`, `LICENSE.novnc` at
`../web/novnc/LICENSE.txt` -- so that updating the dependency updates the
license text with it. Where there is no such file, the entry is a copy.

Add an entry whenever you add a dependency, and remove it when you drop one.

## Bundled web assets

miniweb's front end ships third-party JavaScript and CSS directly in `web/`:
noVNC, xterm.js, the mxGraph editor under `web/grapheditor/`, and the libraries
under `web/libs/` such as D3, DataTables, and jQuery. These are not Go modules,
so they are not in `go.mod` and `go mod vendor` does not touch them. Update
them by replacing the files, and keep their `LICENSES/` entries current.

## See also

- [Contributing and Development Guide](../contributing.md)
- [Releases](releases.md)
