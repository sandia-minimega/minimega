# minimega development container

Use this container for Linux source builds and unit tests when developing on
macOS or Windows. It provides Go 1.24, vendored module resolution, libpcap
headers, and the Python tooling used by the build scripts. It is a development
environment, not the privileged minimega deployment image under `docker/`.

## Start with VS Code

1. Install Docker Desktop, configured to use Linux containers, and the VS Code
   [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers).
2. Open the repository root in VS Code.
3. Run **Dev Containers: Reopen in Container** from the Command Palette.
4. After changing either file in `.devcontainer/`, run
   **Dev Containers: Rebuild Container**.

The repository is bind-mounted into the container, so source edits and generated
files remain on the host.

## Run checks

Open a terminal in the container and initialize the repository environment:

```bash
source scripts/env.bash
go version
```

Run the smallest checks that cover the change:

```bash
./scripts/check.bash
./scripts/test.bash
GOFLAGS=-mod=vendor go test ./pkg/...
git diff --check
```

Use `./scripts/build.bash` or `./scripts/all.bash` when the change requires the
full source build or generated documentation. These scripts create ignored
outputs in the bind-mounted repository; inspect the working tree afterward and
use `./scripts/clean.bash` only when it will not remove wanted work.

The optional
[Dev Container CLI](https://github.com/devcontainers/cli) can run the same
environment without the VS Code UI:

```bash
devcontainer up --workspace-folder .
devcontainer exec --workspace-folder . bash -c 'source scripts/env.bash && ./scripts/test.bash'
```

## Limitations

Docker Desktop supplies Linux user space, but not a Linux host equivalent for
KVM, host kernel modules, cgroups, network namespaces, or Open vSwitch. Do not
use this container as final validation for privileged runtime or minitest
behavior. Use a suitable Linux host, a Linux VM with nested virtualization when
required, or GitHub Actions, and report unverified runtime checks.

Linked Git worktrees may place their common Git directory outside the mounted
workspace. Prefer opening a regular clone. For a worktree created with relative
Git paths, the Dev Container CLI can mount its common directory:

```bash
devcontainer up --workspace-folder . --mount-git-worktree-common-dir
```

If Git still cannot resolve the worktree, use a regular clone or explicitly
mount the common Git directory. Do not rewrite the worktree's `.git` file.
