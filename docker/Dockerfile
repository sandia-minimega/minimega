# To specify an alternate base image: --build-arg BASE_IMAGE="some/other/image:tag"
ARG BASE_IMAGE="ubuntu:22.04"

# -- go builder --
FROM golang:1.18.2 AS gobuilder

ENV DEBIAN_FRONTEND="noninteractive"
RUN apt-get update && apt-get install -y libpcap-dev

WORKDIR /minimega/

COPY . .

RUN ./scripts/all.bash


# -- minimega image --
FROM ${BASE_IMAGE}

# General image metadata
# Reference: https://github.com/opencontainers/image-spec/blob/main/annotations.md
ARG MM_VERSION
# LABEL org.opencontainers.image.authors="minimega dev team <minimega-dev@sandia.gov>" \
#   org.opencontainers.image.documentation="https://www.sandia.gov/minimega/" \
#   org.opencontainers.image.vendor="Sandia National Laboratories" \
#   org.opencontainers.image.licenses="GPL-3.0-only" \


#   "Labels": {
#     "org.opencontainers.image.authors": "minimega dev team \u003cminimega-dev@sandia.gov\u003e",
#     "org.opencontainers.image.created": "2025-02-11T16:13:37.952Z",
#     "org.opencontainers.image.description": "FIREWHEEL is an experiment orchestration tool that assists a user in building and controlling, repeatable experiments of distributed network systems at any scale.",
#     "org.opencontainers.image.documentation": "https://www.sandia.gov/minimega/",
#     "org.opencontainers.image.licenses": "NOASSERTION",
#     "org.opencontainers.image.ref.name": "ubuntu",
#     "org.opencontainers.image.revision": "bbb41714d4b4c428a7d45acee07dae2430c5bff4",
#     "org.opencontainers.image.source": "https://github.com/sdelliot/firewheel",
#     "org.opencontainers.image.title": "firewheel",
#     "org.opencontainers.image.url": "https://github.com/sdelliot/firewheel",
#     "org.opencontainers.image.vendor": "Sandia National Laboratories",
#     "org.opencontainers.image.version": "feat-dockerize"
# }
# },

ENV DEBIAN_FRONTEND="noninteractive"

RUN apt-get update \
  && apt-get install -y \
    dnsmasq iproute2 isc-dhcp-client \
    libpcap-dev ntfs-3g openssh-client \
    openvswitch-switch qemu-kvm qemu-utils \
  && apt autoremove -y \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

COPY --from=gobuilder /minimega/bin/minimega /opt/minimega/bin/minimega
COPY --from=gobuilder /minimega/bin/miniweb  /opt/minimega/bin/miniweb

# The version of miniccc used has to match exactly with the version of minimega
# running, so let's include them in the image so we can easily grab a copy that
# is sure to be the same version.
COPY --from=gobuilder /minimega/bin/miniccc     /opt/minimega/bin/miniccc
COPY --from=gobuilder /minimega/bin/miniccc.exe /opt/minimega/bin/miniccc.exe

# For the sake of consistency, let's go ahead and include protonuke and
# minirouter in the image too so we can easily grab a copy if/when necessary.
COPY --from=gobuilder /minimega/bin/protonuke     /opt/minimega/bin/protonuke
COPY --from=gobuilder /minimega/bin/protonuke.exe /opt/minimega/bin/protonuke.exe
COPY --from=gobuilder /minimega/bin/minirouter    /opt/minimega/bin/minirouter

# As the minimega API changes, so does the minimega.py generated file. Given
# this, let's go ahead and also include the lib directory so we can grab the
# updated Python package from the Docker image.
COPY --from=gobuilder /minimega/lib     /opt/minimega/lib
COPY --from=gobuilder /minimega/README  /opt/minimega/lib/README
COPY --from=gobuilder /minimega/VERSION /opt/minimega/lib/VERSION

COPY ./web       /opt/minimega/web
COPY ./docker/mm /usr/local/bin/mm

COPY ./docker/start-minimega.sh /start-minimega.sh

RUN chmod +x /usr/local/bin/mm \
  && chmod +x /start-minimega.sh

WORKDIR /opt/minimega

CMD ["/start-minimega.sh"]
