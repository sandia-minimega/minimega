# Dependencies


<a id="TOC_1."></a>

## Introduction

minimega uses Go modules. `go.mod` lists the dependencies and their versions,
and `go.sum` records their checksums, so every build uses the same code. The
Go toolchain downloads the modules into the module cache on the first build.
To build without network access, run `go mod download` while online and then
build with the same module cache.

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
- Copy the dependency's license into `LICENSES/LICENSE.<name>`. The Debian and
  RPM packages install that directory.

<a id="TOC_3."></a>

## Patching a dependency

Sometimes we find issues in our dependencies that are not resolved upstream.
Rather than wait for upstream to patch, we keep a patched copy under
`packages/` and point to it with a `replace` directive in `go.mod`:

```
replace github.com/goftp/server => ./packages/github.com/goftp/server
```

Record the upstream commit of each copy in `packages/versions`. Commit the
patches separately from the copy so that they can be reapplied when the
dependency is updated.

<a id="TOC_4."></a>

## Updating a dependency

Run `go get <module>@<version>`, then `go mod tidy`, and commit the changes to
`go.mod` and `go.sum`. Check that the license in `LICENSES/` is still current.

For patched dependencies under `packages/`, you must port the patches to the
new version. Good luck.
