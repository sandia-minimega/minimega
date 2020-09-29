FROM node:14.2 AS jsbuilder

RUN npm install -g @vue/cli

COPY ./src/js /phenix/src/js

WORKDIR /phenix/src/js

ARG PHENIX_WEB_AUTH
ENV VUE_APP_AUTH ${PHENIX_WEB_AUTH:-enabled}

RUN npm install \
  && npm run build


FROM golang:1.14 AS gobuilder

RUN apt update \
  && apt install -y protobuf-compiler xz-utils

COPY ./Makefile /phenix/Makefile
COPY ./src/go   /phenix/src/go

WORKDIR /phenix

COPY --from=jsbuilder /phenix/src/js /phenix/src/js

ARG INSTALL_CERT=
RUN if [ -n "${INSTALL_CERT}" ]; then \
  wget ${INSTALL_CERT} -e use_proxy=no \
    -O /usr/local/share/ca-certificates/custom.crt \
    && update-ca-certificates; fi

ARG PHENIX_VERSION=
ARG PHENIX_COMMIT=
RUN VER=${PHENIX_VERSION} COMMIT=${PHENIX_COMMIT} make bin/phenix

RUN wget https://github.com/glattercj/vmdb2/releases/download/v1.0/vmdb2 -P bin/ \
  && chmod +x bin/vmdb2


FROM ubuntu:20.04

RUN apt update \
  && apt install -y cpio locales nano vim vmdb2 \
  && locale-gen en_US.UTF-8 \
  && apt clean \
  && rm -rf /var/lib/apt/lists/* \
  && rm -rf /var/cache/apt/archives/*

ENV LANG   en_US.UTF-8
ENV LC_ALL en_US.UTF-8

COPY --from=gobuilder /phenix/bin/phenix /usr/local/bin/phenix
COPY --from=gobuilder /phenix/bin/vmdb2  /usr/bin/vmdb2

CMD ["phenix", "help"]
