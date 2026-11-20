# To specify an alternate base image: --build-arg BASE_IMAGE="some/other/base_image:tag"
# To specify an alternate Go image: --build-arg GO_IMAGE="some/other/go_image:tag"
ARG BASE_IMAGE="ubuntu:22.04"
ARG GO_IMAGE="golang:1.24.13"
ARG PYTHON_IMAGE="python:3.13-slim"

# -- go builder --
FROM ${GO_IMAGE} AS gobuilder

ENV DEBIAN_FRONTEND="noninteractive"
RUN apt-get update && apt-get install -y libpcap-dev

WORKDIR /minimega/

COPY . .

RUN ./scripts/all.bash

# Resolve the LICENSES symlinks, which point into web/, as the packages do.
RUN mkdir -p /licenses && cp -rL LICENSE LICENSES /licenses/


# -- documentation builder --
FROM ${PYTHON_IMAGE} AS docbuilder

WORKDIR /minimega/

COPY zensical.toml ./
COPY doc/requirements.txt ./doc/requirements.txt
COPY scripts/zensical_build.py ./scripts/zensical_build.py
COPY doc/pygments_minimega ./doc/pygments_minimega
RUN apt-get update \
  && apt-get install -y --no-install-recommends git \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir -r doc/requirements.txt

COPY --from=gobuilder /minimega/doc/content ./doc/content
COPY --from=gobuilder /minimega/doc/overrides ./doc/overrides
COPY --from=gobuilder /minimega/lib/minimega.py ./lib/minimega.py
COPY --from=gobuilder /minimega/.git ./.git
RUN python scripts/zensical_build.py build --strict


# -- Firecracker download --
# Installs Firecracker and the jailer of FIRECRACKER_VERSION, so the build needs
# network access to github.com. FIRECRACKER_SHA256 is the expected SHA-256 of the
# release tarball; when it is empty, the build uses the .sha256.txt file of the
# release. Change both together. An empty FIRECRACKER_VERSION leaves them out.
FROM ${BASE_IMAGE} AS fcfetch

ARG FIRECRACKER_VERSION="1.17.0"
ARG FIRECRACKER_SHA256="06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558"

WORKDIR /tmp
RUN mkdir -p /fc \
  && if [ -n "${FIRECRACKER_VERSION}" ]; then \
    set -e; \
    export DEBIAN_FRONTEND=noninteractive; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    name="firecracker-v${FIRECRACKER_VERSION}-x86_64.tgz"; \
    url="https://github.com/firecracker-microvm/firecracker/releases/download/v${FIRECRACKER_VERSION}"; \
    curl -fsSLO "${url}/${name}"; \
    if [ -n "${FIRECRACKER_SHA256}" ]; then \
      echo "${FIRECRACKER_SHA256}  ${name}" > "${name}.sha256.txt"; \
    else \
      curl -fsSLO "${url}/${name}.sha256.txt"; \
    fi; \
    sha256sum -c "${name}.sha256.txt"; \
    tar xzf "${name}"; \
    dir="release-v${FIRECRACKER_VERSION}-x86_64"; \
    install -m 0755 "${dir}/firecracker-v${FIRECRACKER_VERSION}-x86_64" /fc/firecracker; \
    install -m 0755 "${dir}/jailer-v${FIRECRACKER_VERSION}-x86_64" /fc/jailer; \
  fi


# -- minimega image --
FROM ${BASE_IMAGE}

ENV DEBIAN_FRONTEND="noninteractive" \
    PATH="$PATH:/opt/minimega/bin"

# acl (setfacl), dmsetup, iproute2, and qemu-utils are for Firecracker VMs.
# libxkbfile1: the Android emulator (vm launch android, with the Android SDK
# mounted into the container) links against it.
# cpio: vmbetter and passwordify unpack and pack initramfs images with it.
RUN apt-get update \
  && apt-get install -y \
    acl bash-completion dmsetup dnsmasq iproute2 isc-dhcp-client \
    cpio less libpcap-dev ntfs-3g openssh-client \
    openvswitch-switch qemu-kvm qemu-utils \
    iptables ffmpeg \
    libxkbfile1 \
  && apt-get autoremove -y \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

# Copy all binaries, including tools such as vmbetter, rfbplay and vncdrone
# While this includes some tools like minitest that generally aren't
# needed in a deployment, the disk utilization difference is minimal,
# and ensures any future tools get included in the Docker image.
COPY --from=gobuilder /minimega/bin/ /opt/minimega/bin/
COPY --from=gobuilder /licenses/ /usr/share/doc/minimega/

# Empty unless the build set FIRECRACKER_VERSION.
COPY --from=fcfetch /fc/ /usr/local/bin/

# Pre-generate the bash tab-completion script so it's available out of the
# box for interactive shells inside the container (e.g. `docker exec -it
# minimega bash`), picked up automatically via the bash-completion package
# installed above.
RUN mkdir -p /usr/share/bash-completion/completions \
  && /opt/minimega/bin/minimega -completion bash > /usr/share/bash-completion/completions/minimega

# As the minimega API changes, so does the minimega.py generated file. Given
# this, let's go ahead and also include the lib directory so we can grab the
# updated Python package from the Docker image.
COPY --from=gobuilder /minimega/lib       /opt/minimega/lib

# Copy files from source tree
COPY ./web /opt/minimega/web
COPY --from=docbuilder /minimega/site /opt/minimega/web/docs
COPY ./docker/mm /usr/local/bin/mm
COPY ./docker/start-minimega.sh /start-minimega.sh

RUN ln -s /opt/minimega/web/docs /opt/minimega/docs \
  && chmod +x /usr/local/bin/mm \
  && chmod +x /start-minimega.sh

WORKDIR /opt/minimega

CMD ["/start-minimega.sh"]
