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

COPY ./misc/web /opt/minimega/misc/web

ADD phenix/docker/minimega/minimega.service /etc/systemd/system/minimega.service
ADD phenix/docker/minimega/miniweb.service  /etc/systemd/system/miniweb.service

WORKDIR /etc/systemd/system/multi-user.target.wants

RUN ln -s ../minimega.service \
  && ln -s ../miniweb.service

ARG MM_BASE=/tmp/minimega
ARG MM_FILEPATH=/phenix/images
ARG MM_DEGREE=20
ARG MM_LOGLEVEL=debug
ARG MM_LOGFILE=/var/log/minimega.log

RUN echo "MM_BASE=${MM_BASE}" >> /etc/default/minimega \
  && echo "MM_FILEPATH=${MM_FILEPATH}" >> /etc/default/minimega \
  && echo "MM_DEGREE=${MM_DEGREE}" >> /etc/default/minimega \
  && echo "MM_LOGLEVEL=${MM_LOGLEVEL}" >> /etc/default/minimega \
  && echo "MM_LOGFILE=${MM_LOGFILE}" >> /etc/default/minimega

WORKDIR /root
