# QEMU build image

`docker/qemu/Dockerfile` builds QEMU from the upstream release tarball. The
result is the image `qemu-build`. The image contains only two paths:

- `/out`: the installed QEMU tree. The prefix is `/usr/local`.
- `/out-packages`: the Ubuntu packages that the installed files need at run time.

The image has no shell and cannot run as a container. Other images copy QEMU
from it.

## Contents

- Emulators: `qemu-system-x86_64`, `qemu-system-i386`, `qemu-system-arm`, and
  `qemu-system-aarch64`.
- Tools: `qemu-img`, `qemu-io`, `qemu-nbd`, and `qemu-storage-daemon`.
- `kvm`: a wrapper that starts `qemu-system-x86_64 -enable-kvm`. Upstream QEMU
  does not install a `kvm` program, and minimega uses that name by default.
- A modular build. QEMU loads the block drivers, SPICE, QXL, and USB
  redirection only when a VM uses them.
- No graphical display backends and no host audio backends.

The `configure` command enables each feature with an explicit option. Thus
`configure` stops when a library for a feature is absent.

## Build the image

```bash
docker build -t qemu-build -f docker/qemu/Dockerfile docker/qemu
```

The build compares the libraries that the installed files load with the
package list in the Dockerfile. The build stops when the two lists are
different. If this occurs, correct the package list in the Dockerfile.

## Published tags

The workflow `.github/workflows/qemu-build.yml` publishes the image as
`ghcr.io/<owner>/qemu-build:<QEMU version>-noble-<build revision>`.

- The workflow runs when a change to `docker/qemu/` merges into `master`.
- The workflow does not replace a published tag. To publish a changed recipe,
  increase `QEMU_BUILD_REVISION`.
- A pull request that changes `docker/qemu/` builds the image and does not
  publish it.

## Change the QEMU version

1. Download the tarball and its signature from <https://download.qemu.org/>.

    ```bash
    curl -fsSLO https://download.qemu.org/qemu-<version>.tar.xz
    curl -fsSLO https://download.qemu.org/qemu-<version>.tar.xz.sig
    ```

2. Get the QEMU release key.

    ```bash
    gpg --keyserver hkps://keys.openpgp.org \
      --recv-keys CEACC9E15534EBABB82D3FA03353C9CEF108B584
    ```

3. Make sure that the signature is good.

    ```bash
    gpg --verify qemu-<version>.tar.xz.sig qemu-<version>.tar.xz
    ```

4. Calculate the checksum of the tarball.

    ```bash
    sha256sum qemu-<version>.tar.xz
    ```

5. In `docker/qemu/Dockerfile`, set `QEMU_VERSION` and `QEMU_SHA256`.
6. Set `QEMU_BUILD_REVISION` to `1`.
7. Build the image. Correct the `configure` options if QEMU changed an option
   name. The file `scripts/meson-buildoptions.sh` in the tarball lists the
   options.
8. Open a pull request with the changes.
