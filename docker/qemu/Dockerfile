# QEMU built from the upstream release tarball for the minimega image.
#   docker build -t qemu-build -f docker/qemu/Dockerfile docker/qemu
# The image holds only /out (the installed tree) and /out-packages (the Ubuntu
# runtime packages that the installed files need).
ARG BASE_IMAGE="ubuntu:24.04"
ARG QEMU_VERSION="11.1.3"
ARG QEMU_SHA256="5a8289c7940dc77d3873c81a6c29ba3afe1446fc9d36e77fa1658cdbecb84b22"
ARG QEMU_BUILD_REVISION="1"

# -- QEMU builder --
FROM ${BASE_IMAGE} AS build

ARG QEMU_VERSION
ARG QEMU_SHA256

ENV DEBIAN_FRONTEND="noninteractive"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    bison \
    build-essential \
    bzip2 \
    flex \
    libaio-dev \
    libattr1-dev \
    libbpf-dev \
    libbz2-dev \
    libcacard-dev \
    libcap-ng-dev \
    libcurl4-gnutls-dev \
    libfdt-dev \
    libfuse3-dev \
    libglib2.0-dev \
    libgnutls28-dev \
    libibverbs-dev \
    libiscsi-dev \
    libjpeg-dev \
    libnfs-dev \
    libnuma-dev \
    libpixman-1-dev \
    libpmem-dev \
    libpng-dev \
    librbd-dev \
    librdmacm-dev \
    libsasl2-dev \
    libseccomp-dev \
    libselinux1-dev \
    libslirp-dev \
    libspice-protocol-dev \
    libspice-server-dev \
    libssh-dev \
    libudev-dev \
    liburing-dev \
    libusb-1.0-0-dev \
    libusbredirparser-dev \
    libzstd-dev \
    ninja-build \
    pkg-config \
    python3 \
    python3-setuptools \
    python3-venv \
    python3-wheel \
    xz-utils \
    zlib1g-dev \
  && rm -rf /var/lib/apt/lists/*

# ADD downloads through the build daemon, so the stage needs no TLS client.
ADD https://download.qemu.org/qemu-${QEMU_VERSION}.tar.xz /src/

WORKDIR /src

RUN echo "${QEMU_SHA256}  qemu-${QEMU_VERSION}.tar.xz" | sha256sum -c - \
  && tar -xJf "qemu-${QEMU_VERSION}.tar.xz"

WORKDIR /src/qemu-${QEMU_VERSION}

# Each wanted feature is enabled explicitly, so configure fails when its
# library is missing instead of silently building without it.
RUN ./configure \
    --prefix=/usr/local \
    --libdir=/usr/local/lib \
    --sysconfdir=/etc \
    --localstatedir=/var \
    --target-list=x86_64-softmmu,i386-softmmu,arm-softmmu,aarch64-softmmu \
    --enable-modules \
    --disable-download \
    --enable-strip \
    --disable-docs \
    --disable-rust \
    --disable-guest-agent \
    --disable-gtk \
    --disable-sdl \
    --disable-sdl-image \
    --disable-opengl \
    --disable-virglrenderer \
    --disable-vte \
    --disable-curses \
    --disable-dbus-display \
    --disable-brlapi \
    --disable-xkbcommon \
    --disable-xen \
    --disable-xen-pci-passthrough \
    --disable-alsa \
    --disable-pa \
    --disable-pipewire \
    --disable-jack \
    --disable-oss \
    --disable-sndio \
    --enable-kvm \
    --enable-tcg \
    --enable-tools \
    --enable-pixman \
    --enable-fdt=system \
    --enable-slirp \
    --enable-vnc \
    --enable-vnc-jpeg \
    --enable-vnc-sasl \
    --enable-png \
    --enable-spice \
    --enable-spice-protocol \
    --enable-gnutls \
    --enable-seccomp \
    --enable-cap-ng \
    --enable-selinux \
    --enable-tpm \
    --enable-attr \
    --enable-virtfs \
    --enable-linux-aio \
    --enable-linux-io-uring \
    --enable-curl \
    --enable-libiscsi \
    --enable-libnfs \
    --enable-libssh \
    --enable-rbd \
    --enable-bzip2 \
    --enable-zstd \
    --enable-fuse \
    --enable-libpmem \
    --enable-libusb \
    --enable-usb-redir \
    --enable-smartcard \
    --enable-numa \
    --enable-rdma \
    --enable-bpf \
    --enable-libudev \
    --enable-vhost-net \
    --enable-vhost-kernel \
    --enable-vhost-user \
    --enable-vhost-vdpa

RUN make -j"$(nproc)" \
  && make install DESTDIR=/out

# Map each shared library that an installed program or module loads to its
# Ubuntu package, and fail when the result differs from the reviewed list.
RUN find /out/usr/local/bin /out/usr/local/libexec /out/usr/local/lib/qemu -type f -print0 \
    | while IFS= read -r -d '' f; do ldd "$f" 2>/dev/null || true; done > /tmp/ldd.txt \
  && if grep 'not found' /tmp/ldd.txt; then exit 1; fi \
  && awk '$2 == "=>" && $3 ~ /^\// {print $3}' /tmp/ldd.txt | sort -u \
    | while read -r lib; do \
        pkg=""; \
        for p in "$lib" "$(readlink -f "$lib")" "/usr$lib" "$(readlink -f "/usr$lib")"; do \
          pkg="$(dpkg-query -S "$p" 2>/dev/null | awk -F: 'NR == 1 {print $1}')" && [ -n "$pkg" ] && break; \
        done; \
        [ -n "$pkg" ] || { echo "no package owns ${lib}" >&2; exit 1; }; \
        echo "$pkg"; \
      done | sort -u > /tmp/derived.txt \
  && printf '%s\n' \
    libaio1t64 \
    libargon2-1 \
    libblkid1 \
    libboost-iostreams1.83.0 \
    libboost-thread1.83.0 \
    libbpf1 \
    libbrotli1 \
    libbz2-1.0 \
    libc6 \
    libcacard0 \
    libcap-ng0 \
    libcap2 \
    libcom-err2 \
    libcryptsetup12 \
    libcurl3t64-gnutls \
    libdaxctl1 \
    libdevmapper1.02.1 \
    libdw1t64 \
    libelf1t64 \
    libfdt1 \
    libffi8 \
    libfuse3-3 \
    libgcc-s1 \
    libglib2.0-0t64 \
    libgmp10 \
    libgnutls30t64 \
    libgssapi-krb5-2 \
    libgstreamer-plugins-base1.0-0 \
    libgstreamer1.0-0 \
    libhogweed6t64 \
    libibverbs1 \
    libidn2-0 \
    libiscsi7 \
    libjpeg-turbo8 \
    libjson-c5 \
    libk5crypto3 \
    libkeyutils1 \
    libkmod2 \
    libkrb5-3 \
    libkrb5support0 \
    libldap2 \
    liblz4-1 \
    liblzma5 \
    libmount1 \
    libndctl6 \
    libnettle8t64 \
    libnfs14 \
    libnghttp2-14 \
    libnl-3-200 \
    libnl-route-3-200 \
    libnspr4 \
    libnss3 \
    libnuma1 \
    libopus0 \
    liborc-0.4-0t64 \
    libp11-kit0 \
    libpcre2-8-0 \
    libpcsclite1 \
    libpixman-1-0 \
    libpmem1 \
    libpng16-16t64 \
    libpsl5t64 \
    librados2 \
    librbd1 \
    librdmacm1t64 \
    librtmp1 \
    libsasl2-2 \
    libseccomp2 \
    libselinux1 \
    libslirp0 \
    libspice-server1 \
    libssh-4 \
    libssl3t64 \
    libstdc++6 \
    libtasn1-6 \
    libudev1 \
    libunistring5 \
    libunwind8 \
    liburing2 \
    libusb-1.0-0 \
    libusbredirparser1t64 \
    libuuid1 \
    libzstd1 \
    zlib1g \
    > /tmp/expected.txt \
  && diff -u /tmp/expected.txt /tmp/derived.txt \
  && paste -sd ' ' /tmp/expected.txt > /out-packages

COPY --chmod=0755 kvm /out/usr/local/bin/kvm


# -- QEMU image --
FROM scratch

ARG QEMU_VERSION
ARG QEMU_BUILD_REVISION

LABEL org.opencontainers.image.title="qemu-build" \
      org.opencontainers.image.description="QEMU built from source for the minimega image" \
      org.opencontainers.image.source="https://github.com/sandia-minimega/minimega" \
      org.opencontainers.image.licenses="GPL-2.0-only" \
      org.opencontainers.image.version="${QEMU_VERSION}" \
      org.opencontainers.image.revision="${QEMU_BUILD_REVISION}"

COPY --from=build /out /out
COPY --from=build /out-packages /out-packages
