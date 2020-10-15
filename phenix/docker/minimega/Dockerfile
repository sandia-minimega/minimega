FROM golang:1.12.17 AS gobuilder

RUN apt-get update && apt-get install -y libpcap-dev

WORKDIR /minimega/

COPY ./.git   ./.git
COPY ./*.bash ./
COPY ./src    ./src

RUN ./build.bash


FROM jrei/systemd-ubuntu:18.04

RUN apt update && apt install -y \
  dnsmasq iproute2 isc-dhcp-client \
  libpcap-dev ntfs-3g openssh-client \
  openvswitch-switch qemu-kvm qemu-utils \
  && apt autoremove -y \
  && apt clean -y\
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

COPY --from=gobuilder /minimega/bin/minimega /usr/bin/minimega
COPY --from=gobuilder /minimega/bin/miniweb  /usr/bin/miniweb

# The version of miniccc used has to match exactly with the version of minimega
# running, so let's include them in the image so we can easily grab a copy that
# is sure to be the same version.
COPY --from=gobuilder /minimega/bin/miniccc     /opt/minimega/bin/miniccc
COPY --from=gobuilder /minimega/bin/miniccc.exe /opt/minimega/bin/miniccc.exe

# For the sake of consistency, let's go ahead and include protonuke in the image
# too so we can easily grab a copy if/when necessary.
COPY --from=gobuilder /minimega/bin/protonuke     /opt/minimega/bin/protonuke
COPY --from=gobuilder /minimega/bin/protonuke.exe /opt/minimega/bin/protonuke.exe

COPY ./misc/web /opt/minimega/misc/web

ADD phenix/docker/minimega/minimega.service /etc/systemd/system/minimega.service
ADD phenix/docker/minimega/miniweb.service  /etc/systemd/system/miniweb.service

WORKDIR /etc/systemd/system/multi-user.target.wants

RUN ln -s ../minimega.service \
  && ln -s ../miniweb.service

ARG MM_BASE=/tmp/minimega
ARG MM_FILEPATH=/phenix/images
ARG MM_PORT=9000
ARG MM_DEGREE=20
ARG MM_CONTEXT=phenix
ARG MM_LOGLEVEL=debug
ARG MM_LOGFILE=/var/log/minimega.log

RUN echo "MM_BASE=${MM_BASE}" >> /etc/default/minimega \
  && echo "MM_FILEPATH=${MM_FILEPATH}" >> /etc/default/minimega \
  && echo "MM_PORT=${MM_PORT}" >> /etc/default/minimega \
  && echo "MM_DEGREE=${MM_DEGREE}" >> /etc/default/minimega \
  && echo "MM_CONTEXT=${MM_CONTEXT}" >> /etc/default/minimega \
  && echo "MM_LOGLEVEL=${MM_LOGLEVEL}" >> /etc/default/minimega \
  && echo "MM_LOGFILE=${MM_LOGFILE}" >> /etc/default/minimega

WORKDIR /root
