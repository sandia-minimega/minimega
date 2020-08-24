FROM ubuntu:20.04

ENV TZ=America/Denver
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

RUN apt update \
  && apt install -y build-essential curl git gnupg2 make protobuf-compiler wget xz-utils

ENV GOLANG_VERSION 1.14.6

RUN wget -O go.tgz https://golang.org/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz \
  && tar -C /usr/local -xzf go.tgz && rm go.tgz

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

RUN GO111MODULE=on go get -v github.com/go-delve/delve/cmd/dlv@latest 2>&1
RUN chmod -R a+rwX /go/pkg && rm -rf /go/src/*

ENV NODE_VERSION 12.18.3

RUN wget -O node.txz https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz \
  && tar -xJf node.txz -C /usr/local --strip-components=1 --no-same-owner && rm node.txz \
  && ln -s /usr/local/bin/node /usr/local/bin/nodejs

RUN npm install -g @vue/cli redoc-cli

RUN curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - \
  && echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list \
  && apt update && apt install -y yarn

ADD . /go/src/phenix

WORKDIR /go/src/phenix

RUN go mod download
RUN make generate-bindata && make generate-protobuf

EXPOSE 2345
EXPOSE 3000

ENTRYPOINT ["dlv", "debug", "--headless", "--listen=:2345", "--log", "--api-version=2"]

CMD ["--", "ui", "--jwt-signing-key=a"]