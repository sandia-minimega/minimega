FROM golang:1.18.2 AS gobuilder

RUN apt-get update && apt-get install -y libpcap-dev

WORKDIR /minimega/

COPY . .

# COPY ./.git     ./.git
# COPY ./scripts  ./scripts
# COPY ./cmd      ./cmd
# COPY ./pkg      ./pkg
# COPY ./internal ./internal
# COPY ./packages ./packages
# COPY ./go.mod   ./go.mod
# COPY ./go.sum   ./go.sum

RUN ./scripts/all.bash


FROM ubuntu:20.04

RUN apt update && apt install -y \
  dnsmasq iproute2 isc-dhcp-client \
  libpcap-dev ntfs-3g openssh-client \
  openvswitch-switch qemu-kvm qemu-utils \
  && apt autoremove -y \
  && apt clean -y\
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
COPY --from=gobuilder /minimega/lib    /opt/minimega/lib
COPY --from=gobuilder /minimega/README /opt/minimega/lib/README

COPY ./web       /opt/minimega/web
COPY ./docker/mm /usr/local/bin/mm

COPY ./docker/start-minimega.sh /start-minimega.sh

RUN chmod +x /usr/local/bin/mm \
  && chmod +x /start-minimega.sh

WORKDIR /opt/minimega

CMD ["/start-minimega.sh"]
