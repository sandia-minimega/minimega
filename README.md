minimega
========

minimega is a tool for launching and managing virtual machines. It can run on
your laptop or distributed across a cluster. minimega is fast, easy to deploy,
and can scale to run on massive clusters with virtually no setup.

See the [minimega documentation](https://sandia-minimega.github.io/minimega/)
for installation, usage, API, and training guides.

## Quickstart

Requirements:
- 64-bit host with KVM and Ubuntu 22.04+
- 12GB of free memory
- Docker and Docker Compose
- An Ubuntu VM disk image named `noble.qc2` (if you don't have one, grab one from [sceptre-phenix-images](https://github.com/sandialabs/sceptre-phenix-images))

Setup:

```shell
git clone https://github.com/sandia-minimega/minimega.git
cd minimega

# Start minimega daemon
sudo docker compose -f docker/docker-compose.yml up -d

sudo install -m 0755 docker/minimega-wrapper.sh /usr/local/bin/minimega  # convenience wrapper
sudo minimega -version  # make sure it works!

# Assuming noble.qc2 is in your home directory
sudo mkdir -p /tmp/minimega/files
sudo cp ~/noble.qc2 /tmp/minimega/files
```

Connect to minimega shell, then configure and launch 10 VMs:

```shell
sudo minimega -attach  # Attach to running daemon

vm config disks noble.qc2  # Relative paths are /tmp/minimega/files with the Docker Compose configuration
vm config memory 1024  # 1GB RAM per VM
vm config vcpus 1  # Single CPU per VM
vm config net LAN WAN MGMT  # 3 interfaces attached to 3 separate VLANs (with named aliases)
vm launch kvm vm[0-9]  # 10 VMs, named vm0 to vm9. They aren't booted yet.
vm start all  # Boot all VMs

# See the VMs with "vm info", with output limited to a subset of columns
.columns name,state,ip,snapshot,cc_active vm info
```

Congrats, you now have 10 running VMs! Next read the [quickstart guide](https://sandia-minimega.github.io/minimega/articles/quickstart/) and follow the [miniclass course](https://sandia-minimega.github.io/minimega/training/miniclass/).

Cleanup the VMs (still in the minimega shell):

```shell
vm kill all  # stop the VMs
vm flush  # remove the stopped VMs
disconnect
```

Then cleanly stop the container:

```shell
sudo docker compose -f docker/docker-compose.yml down
```

## Contributing

This repository uses [prek](https://prek.j178.dev/) (a Rust drop-in alternative
to `pre-commit`) to enforce repository-wide checks (shell linting, YAML
linting, Dockerfile linting, GitHub Actions workflow linting, EditorConfig
rules, spell-checking, conventional commit messages, and general hygiene). The same checks run in CI via the
[Lint workflow](.github/workflows/lint.yml).

Install the dev tooling and register the git `pre-commit` and `commit-msg` hooks
once:

```bash
make install-dev
```

Run all hooks against every file manually:

```bash
make lint
# or, equivalently
prek run --all-files
```

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for additional contribution
guidelines, including conventional commit conventions and EditorConfig setup.
