# Dependencies

minimega is a single Go module, `github.com/sandia-minimega/minimega/v2`, that
requires Go 1.24 or newer. Dependencies are declared with Go modules: `go.mod`
lists them and their versions, and `go.sum` records their checksums, so every
build on every machine compiles the same source. Local forks let us carry
fixes without waiting for them to be resolved upstream, and the build keeps a
license record for everything we ship.

## How dependencies are tracked

| File or directory | What it holds |
|---|---|
| `go.mod` | The module path, the Go version, the direct requirements, the indirect requirements (marked `// indirect`), and the `replace` directives for local forks. |
| `go.sum` | Checksums for every module version the build may use. |
| `packages/` | Local forks of dependencies, wired in with `replace`. See [Local forks](#local-forks). |
| `LICENSES/` | The license of every third-party component. See [Licenses](#licenses). |

## Building offline

The Go toolchain downloads the modules into the module cache on the first
build. To build without network access, run `go mod download` and
`./scripts/licenses.bash` while online, which also caches `go-licenses`, and
then build with the same module cache.

## Adding a dependency

Before adding one, consider:

- Do we actually **need** it? For small amounts of functionality we prefer to
  reimplement rather than take on a dependency.
- What does it pull in with it? An indirect requirement is still a dependency
  we ship and have to keep current. Does it bring a custom logging framework?
- Is the license compatible? minimega is GPLv3; see
  [Copyright](../contributing.md#copyright).

Ask the team if you are unsure -- it never hurts to get a second (or third)
opinion.

To add one:

```bash
go get example.com/some/module@v1.2.3
go mod tidy
./scripts/licenses.bash
```

Prefer a tagged release over a pseudo-version on an untagged commit.
`./scripts/licenses.bash` checks that the build accepts the dependency's
license (see [Licenses](#licenses)). Then run the tests, and commit `go.mod`
and `go.sum` together, as one commit.

## Updating a dependency

```bash
go get example.com/some/module@v1.3.0
go mod tidy
```

Use `go list -m -u all` to see what has newer versions available. Update one
module per commit where you can, so a bad update is easy to revert. If the
module is one of the local forks below, port the fork's changes to the new
code first. Good luck.

## Local forks

When a dependency needs a change that is not resolved upstream, the change
lives in `packages/<import path>/` and a `replace` directive in `go.mod`
points the import at it, so the import path in the code never changes:

```
replace github.com/goftp/server => ./packages/github.com/goftp/server
```

| Import path | Replaced by |
|---|---|
| `github.com/dutchcoders/goftp` | `./packages/github.com/dutchcoders/goftp` |
| `github.com/goftp/server` | `./packages/github.com/goftp/server` |
| `github.com/stargrave/goircd` | `./packages/github.com/stargrave/goircd` |
| `github.com/thoj/go-ircevent` | `./packages/github.com/thoj/go-ircevent` |
| `github.com/Harvey-OS/ninep` | `github.com/jcrussell/ninep`, a fork hosted on GitHub rather than a local copy |

Keep the upstream license file in each copy, named so that `go-licenses`
finds it (for example `LICENSE` or `COPYING`). Record the upstream commit of
each copy in `packages/versions`. Commit the patches separately from the copy
so that they can be reapplied when the dependency is updated.

## Licenses

`scripts/build.bash` runs `scripts/licenses.bash`, which uses
[`go-licenses`](https://github.com/google/go-licenses) to collect the license
of every Go module that the programs link into `LICENSES/go/`. That directory
is generated, so do not commit it. The build fails if a module has no license
file that `go-licenses` recognizes, or a license that is not in
`ALLOWED_LICENSES` in the script. Add a license to that list only after
checking that it is compatible with the GPLv3.

`go-licenses` sees only the Go modules in `go.mod`, so the other files in
`LICENSES/` are maintained by hand: the Go standard library, and code that is
copied into the repository, such as the JavaScript libraries in `web/`. When
you add or update such code, add or update its license in `LICENSES/`, either
as a copy or as a symbolic link to the license file that the code includes
(`LICENSE.novnc` points at `../web/novnc/LICENSE.txt`), so that updating the
code updates the license text with it.

The Debian and RPM packages and the container image install `LICENSE` and
`LICENSES/` in `/usr/share/doc/minimega/`.

## Bundled web assets

miniweb's front end ships third-party JavaScript and CSS directly in `web/`:
noVNC, xterm.js, the mxGraph editor under `web/grapheditor/`, and the libraries
under `web/libs/` such as D3, DataTables, and jQuery. These are not Go modules,
so they are not in `go.mod`. Update them by replacing the files, and keep
their `LICENSES/` entries current.

## See also

- [Contributing and Development Guide](../contributing.md)
- [Releases](releases.md)
