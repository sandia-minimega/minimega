# Dependencies


<a id="TOC_1."></a>

## Introduction

minimega uses Go modules. `go.mod` lists the dependencies and their versions,
and `go.sum` records their checksums, so every build uses the same code. The
Go toolchain downloads the modules into the module cache on the first build.
To build without network access, run `go mod download` and
`./scripts/licenses.bash` while online, which also caches `go-licenses`, and
then build with the same module cache.

<a id="TOC_2."></a>

## Adding a new dependency

When you need to add a new dependency, there are a few things to consider:

- Do we actually **need** to add a new dependency? If it is not too complicated functionality, we prefer to reimplement.
- Does the dependency also have dependencies? Does it bring a custom logging framework?
- Does the dependency have a compatible license?

Ask the team if you are unsure -- it never hurts to get a second (or third)
opinion.

To add a dependency:

- Run `go get <module>@<version>`. Use a tagged release where one exists.
- Run `go mod tidy` and commit the changes to `go.mod` and `go.sum`.
- Run `./scripts/licenses.bash` to check that the build accepts the
  dependency's license (see [Licenses](#TOC_5.)).

<a id="TOC_3."></a>

## Patching a dependency

Sometimes we find issues in our dependencies that are not resolved upstream.
Rather than wait for upstream to patch, we keep a patched copy under
`packages/` and point to it with a `replace` directive in `go.mod`:

```
replace github.com/goftp/server => ./packages/github.com/goftp/server
```

Keep the upstream license file in the copy, named so that `go-licenses` finds
it (for example `LICENSE` or `COPYING`). Record the upstream commit of each
copy in `packages/versions`. Commit the patches separately from the copy so
that they can be reapplied when the dependency is updated.

<a id="TOC_4."></a>

## Updating a dependency

Run `go get <module>@<version>`, then `go mod tidy`, and commit the changes to
`go.mod` and `go.sum`.

For patched dependencies under `packages/`, you must port the patches to the
new version. Good luck.

<a id="TOC_5."></a>

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
as a copy or as a symlink to the license file that the code includes.

The Debian and RPM packages and the container image install `LICENSE` and
`LICENSES/` in `/usr/share/doc/minimega/`.
