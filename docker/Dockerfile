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


# -- miniweb image --
FROM ${BASE_IMAGE} AS miniweb

ENV DEBIAN_FRONTEND="noninteractive" \
    PATH="$PATH:/opt/minimega/bin"

# miniweb serves static files and proxies to minimega over its Unix socket. The
# only package it needs is libpcap, which the minimega binary below links
# against; miniweb itself needs nothing beyond the base image.
RUN apt-get update \
  && apt-get install -y --no-install-recommends libpcap0.8 \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

COPY --from=gobuilder /minimega/bin/miniweb /opt/minimega/bin/miniweb

# The minimega binary is included so the web console can work (MINIWEB_CONSOLE).
# miniweb spawns it as `minimega -attach`, which is a client that talks to the
# minimega container over the shared command socket.
COPY --from=gobuilder /minimega/bin/minimega /opt/minimega/bin/minimega

# Copy files from source tree
COPY ./web /opt/minimega/web
COPY --from=docbuilder /minimega/site /opt/minimega/web/docs
COPY ./docker/load-defaults.sh /load-defaults.sh
COPY ./docker/start-miniweb.sh /start-miniweb.sh
COPY ./docker/console-attach.sh /console-attach.sh

RUN ln -s /opt/minimega/web/docs /opt/minimega/docs \
  && chmod +x /start-miniweb.sh /console-attach.sh

WORKDIR /opt/minimega

CMD ["/start-miniweb.sh"]


# -- minimega image --
FROM ${BASE_IMAGE} AS minimega

ENV DEBIAN_FRONTEND="noninteractive" \
    PATH="$PATH:/opt/minimega/bin"

RUN apt-get update \
  && apt-get install -y \
    bash-completion dnsmasq iproute2 isc-dhcp-client \
    less libpcap-dev ntfs-3g openssh-client \
    openvswitch-switch qemu-kvm qemu-utils \
    iptables ffmpeg \
  && apt autoremove -y \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

# Copy all binaries, including tools such as vmbetter, rfbplay and vncdrone
# While this includes some tools like minitest that generally aren't
# needed in a deployment, the disk utilization difference is minimal,
# and ensures any future tools get included in the Docker image.
# miniweb is included for the same reason, but it is not started here; it
# runs in its own container built from the miniweb stage above.
COPY --from=gobuilder /minimega/bin/ /opt/minimega/bin/

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
COPY --from=gobuilder /minimega/README.md /opt/minimega/lib/README.md
COPY --from=gobuilder /minimega/VERSION   /opt/minimega/lib/VERSION

# Copy files from source tree
COPY ./docker/mm /usr/local/bin/mm
COPY ./docker/load-defaults.sh /load-defaults.sh
COPY ./docker/start-minimega.sh /start-minimega.sh

RUN chmod +x /usr/local/bin/mm \
  && chmod +x /start-minimega.sh

WORKDIR /opt/minimega

CMD ["/start-minimega.sh"]
