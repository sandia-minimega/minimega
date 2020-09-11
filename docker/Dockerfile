FROM golang:1.12 AS gobuilder
RUN apt-get update && apt-get install -y libpcap-dev
WORKDIR /root/
RUN git clone https://github.com/sandia-minimega/minimega.git
WORKDIR /root/minimega/
RUN ./build.bash

FROM jrei/systemd-ubuntu:18.04
RUN apt-get update && apt-get install -y \
  dnsmasq \
  iproute2 \
  isc-dhcp-client \
  libpcap-dev \
  ntfs-3g \
  openssh-client \
  openvswitch-switch \
  qemu-kvm \
  qemu-utils \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*
COPY --from=gobuilder /root/minimega/bin/minimega /usr/local/bin/minimega
COPY --from=gobuilder /root/minimega/bin/miniweb /usr/local/bin/miniweb
COPY --from=gobuilder /root/minimega/bin/pyapigen /usr/local/bin/pyapigen
COPY --from=gobuilder /root/minimega/bin/miniccc /usr/local/bin/miniccc
COPY --from=gobuilder /root/minimega/bin/miniccc.exe /usr/local/bin/miniccc.exe
COPY --from=gobuilder /root/minimega/misc/web /misc/web
CMD ["minimega", "-version"]
