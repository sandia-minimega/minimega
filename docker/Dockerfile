# To specify an alternate base image: --build-arg BASE_IMAGE="some/other/image:tag"
ARG BASE_IMAGE="ubuntu:22.04"

# -- go builder --
FROM golang:1.20.14 AS gobuilder

ENV DEBIAN_FRONTEND="noninteractive"
RUN apt-get update && apt-get install -y libpcap-dev

WORKDIR /minimega/

COPY . .

RUN ./scripts/all.bash


# -- minimega image --
FROM ${BASE_IMAGE}

ENV DEBIAN_FRONTEND="noninteractive" \
    PATH="$PATH:/opt/minimega/bin"

RUN apt-get update \
  && apt-get install -y \
    dnsmasq iproute2 isc-dhcp-client \
    libpcap-dev ntfs-3g openssh-client \
    openvswitch-switch qemu-kvm qemu-utils \
    iptables \
  && apt autoremove -y \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

# Copy all binaries, including tools such as vmbetter, rfbplay and vncdrone
# While this includes some tools like minitest that generally aren't 
# needed in a deployment, the disk utilization difference is minimal, 
# and ensures any future tools get included in the Docker image.
COPY --from=gobuilder /minimega/bin/ /opt/minimega/bin/

# As the minimega API changes, so does the minimega.py generated file. Given
# this, let's go ahead and also include the lib directory so we can grab the
# updated Python package from the Docker image.
COPY --from=gobuilder /minimega/lib       /opt/minimega/lib
COPY --from=gobuilder /minimega/README.md /opt/minimega/lib/README.md
COPY --from=gobuilder /minimega/VERSION   /opt/minimega/lib/VERSION

# Copy files from source tree
COPY ./web /opt/minimega/web
COPY ./docker/mm /usr/local/bin/mm
COPY ./docker/start-minimega.sh /start-minimega.sh

RUN chmod +x /usr/local/bin/mm \
  && chmod +x /start-minimega.sh

WORKDIR /opt/minimega

CMD ["/start-minimega.sh"]
