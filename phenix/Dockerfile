FROM node:14.2 AS jsbuilder

RUN npm install -g @vue/cli

COPY ./src/js /phenix/src/js

WORKDIR /phenix/src/js

ARG PHENIX_WEB_AUTH
ENV VUE_APP_AUTH ${PHENIX_WEB_AUTH:-enabled}

RUN make dist/index.html


FROM golang:1.14 AS gobuilder

RUN apt update \
  && apt install -y protobuf-compiler xz-utils

COPY ./Makefile /phenix/Makefile
COPY ./src/go   /phenix/src/go

WORKDIR /phenix

COPY --from=jsbuilder /phenix/src/js /phenix/src/js

ARG PHENIX_VERSION=

RUN VER=${PHENIX_VERSION} make bin/phenix


FROM ubuntu:20.04

COPY --from=gobuilder /phenix/bin/phenix /usr/bin/phenix

CMD ["phenix", "help"]
