# AGENTS.md

## Project and scoped guidance

minimega is a Go platform for managing KVM virtual machines, Android emulators,
Linux containers, networks, and distributed experiments. The root module is
`github.com/sandia-minimega/minimega/v2`, requires Go 1.24 or newer, and uses
vendored dependencies through `GOFLAGS=-mod=vendor`.

Read the guide that matches the task instead of rediscovering repository
behavior:

- Read [`skills/minimega/SKILL.md`](skills/minimega/SKILL.md) before answering
  minimega usage questions or changing startup flags, CLI commands, namespaces,
  VM lifecycle, networking, file transfer, miniccc, miniweb, Docker or systemd
  configuration, or phēnix/FIREWHEEL integration. See
  [Development and validation](#development-and-validation) for minitest.
- Read [`.devcontainer/README.md`](.devcontainer/README.md) before building or
  testing on macOS or Windows.
- Read the relevant article in
  [`doc/content/articles/developer/`](doc/content/articles/developer/) before
  changing locking, namespaces, naming, status updates, or vendored dependencies.
- Read [`.github/CONTRIBUTING.md`](.github/CONTRIBUTING.md) before opening or
  updating a pull request.

Treat the implementation as authoritative. Update task-specific guidance when a
behavior change makes it stale.

## Repository map

- `cmd/`
  - [`cmd/minimega/`](cmd/minimega/) contains the main daemon, CLI handlers,
    namespaces, scheduling, and VM lifecycle.
  - Companion binaries are `apigen`, `miniccc`, `minifuzzer`,
    `minirouter`, `minitest`, `miniweb`, `nfcat`, `passwordify`, `powerbot`,
    `protonuke`, `pyapigen`, `rfbplay`, `rond`, `vmbetter`, `vmconfiger`, and
    `vncdrone`.
  - [`cmd/plumbing/`](cmd/plumbing/) contains support commands excluded from
    canonical build and test loops.
- [`internal/`](internal/) contains private packages: `bridge`, `gonetflow`,
  `iomeshage`, `meshage`, `miniplumber`, `minitunnel`, `nbd`, `present`, `qemu`,
  `qmp`, `recovery`, `ron`, `version`, `vlans`, `vmconfig`, and `vnc`.
- [`pkg/`](pkg/) contains reusable packages: `minicli`, `miniclient`, `minilog`,
  `minipager`, and `ranges`.
- [`tests/`](tests/) contains minitest command files and matching `.want`
  expectations, including distributed tests.
- [`scripts/`](scripts/) contains canonical check, build, test, documentation,
  install, and cleanup scripts.
- [`doc/content/`](doc/content/) contains Markdown articles, presentations,
  training material, and static content.
- [`.devcontainer/`](.devcontainer/) provides Linux source-build and unit-test
  tooling for macOS and Windows hosts.
- [`docker/`](docker/), [`misc/daemon/`](misc/daemon/), and
  [`packaging/`](packaging/) contain container, service, Debian, and RPM
  configuration.
- [`packages/`](packages/) contains maintained local module forks;
  [`vendor/`](vendor/) contains dependency copies; [`web/`](web/) includes
  bundled third-party projects.
- [`lib/`](lib/) contains Python package metadata; `lib/minimega.py` is
  generated.

Useful references are the
[hosted documentation](https://sandia-minimega.github.io/minimega/), the
[generated API documentation](https://sandia-minimega.github.io/minimega/reference/minimega/),
[`doc/content/articles/installing.md`](doc/content/articles/installing.md),
and [`doc/content/articles/usage.md`](doc/content/articles/usage.md).

## Development and validation

Use a native Linux host for privileged runtime tests. On macOS or Windows, use
the repository development container for Linux builds and unit tests; do not run
the Linux-oriented scripts directly on the host.

Initialize each shell:

```bash
source scripts/env.bash
go version
```

During iteration, run the narrowest relevant test:

```bash
GOFLAGS=-mod=vendor go test ./pkg/minicli
GOFLAGS=-mod=vendor go test ./cmd/minimega -run '^TestName$'
GOFLAGS=-mod=vendor go test -race ./cmd/minimega -run '^TestName$'
```

Use `-race` for concurrency, namespace, meshage, and VM lifecycle changes.
Before submitting a Go change, run:

```bash
./scripts/check.bash
./scripts/test.bash
git diff --check
```

Run `./scripts/all.bash` when the change affects full builds or generated
documentation. The scripts assume GNU/Linux and regenerate ignored binaries,
version metadata, generated API Markdown, Python bindings, and distributions; inspect the
worktree afterward and clean only unwanted outputs.

### minitest functional tests

Run minitest only on a suitable isolated Linux host. Each test is an
extensionless command file under [`tests/`](tests/) with a matching `.want`
file; the runner writes ignored `.got` output.

```bash
sudo ./bin/minitest -dir tests -run '^vm_uuid$'
diff -u tests/vm_uuid.want tests/vm_uuid.got
```

An expectation mismatch prints `got != want` but does not produce a failing exit
status. Automation must inspect the output or explicitly compare `.got` and
`.want`. Use `.filter`, `.columns`, and `.annotate false` to remove unstable
fields, and never accept `.got` without reviewing it.

## Change conventions

- Run `gofmt` on changed Go files and follow nearby package and test patterns.
- Preserve lowercase project names in prose.
- Update CLI help, API documentation, articles, examples, and integration
  guidance when user-visible behavior changes.
- Keep command registration, handlers, generated API Markdown, Python bindings,
  and external callers aligned when command syntax or output changes.
- Do not directly edit `cmd/minimega/vmconfiger_cli.go`,
  `internal/version/version.go`, generated API Markdown, or `lib/minimega.py`.
  Regenerate API Markdown and Python bindings with `./scripts/doc.bash` after
  building the documentation tools.
  Regenerate VM configuration handlers with:

  ```bash
  source scripts/env.bash
  go install ./cmd/vmconfiger
  go generate ./cmd/minimega
  ```

- Files under [`doc/content/`](doc/content/) use Markdown for documentation
  content.
- Files under [`doc/content/releases/`](doc/content/releases/) and
  [`doc/content/presentations/`](doc/content/presentations/) are legacy
  documentation; do not modify or update them.
- Do not run `go mod tidy`, regenerate `vendor/`, or change dependencies unless
  dependency maintenance is the task. Keep changed local forks under `packages/`
  aligned with their `vendor/` copies and preserve license notices.
- Surface invalid configuration and operational errors through existing response
  and logging patterns; do not add silent fallbacks.
- Never add credentials, keys, proprietary data, VM images, packet captures, or
  other sensitive artifacts.

## Concurrency and command invariants

When changing [`cmd/minimega/`](cmd/minimega/):

- Route asynchronous command execution through `runCommands`; do not call
  `minicli.ProcessCommand` directly.
- Acquire locks in this order: `cmdChannel`, `vmLock`, `VM.lock`, then other
  locks. minimega locks precede locks in other packages.
- Use exported `VMs` and namespace helpers instead of directly accessing global
  maps or namespace state.
- Lowercase helpers may require an existing lock; preserve `// LOCK: ...`
  annotations at non-obvious call sites.
- Do not call `ron` operations while holding a VM lock except where existing
  code explicitly documents safety.
- Use the existing status-update mechanism and `meshageStatusPeriod` for
  long-running distributed work.

## CI, releases, and pull requests

When build inputs, generated outputs, dependencies, or packaging change, update
every affected workflow under [`.github/workflows/`](.github/workflows/). Do not
publish artifacts, change `VERSION`, create tags, or use release credentials
unless explicitly requested.

- Follow [`.github/CONTRIBUTING.md`](.github/CONTRIBUTING.md) and
  [`.github/pull_request_template.md`](.github/pull_request_template.md).
- Use `type(scope): subject` for commit messages and pull request titles.
- Use slash-free branch names such as `feat-add-user-authentication`.
- Rebase onto upstream `master`; most pull requests should contain one commit.
  Amend updates and use `--force-with-lease`, never `--force`.
- Add a `Co-authored-by` trailer for each contributing AI agent.
- Keep pull requests focused, update tests and documentation, self-review for
  sensitive data, and report exact validation.
