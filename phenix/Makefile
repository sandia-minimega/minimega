SHELL := /bin/bash

# Default hyperdark version number to the shorthand git commit hash if
# not set at the command line.
VER     := $(or $(VER),$(shell git log -1 --format="%h"))
COMMIT  := $(shell git log -1 --format="%h - %ae")
DATE    := $(shell date -u)
VERSION := $(VER) (commit $(COMMIT)) $(DATE)

GOSOURCES := $(shell find . \( -name '*.go' \))
UISOURCES := $(shell find ./web/ui/src \( -name '*.js' -o -name '*.vue' \))
CONFIGS   := $(shell find api/config/default \( -name '*' \))
TEMPLATES := $(shell find tmpl/templates \( -name '*' \))

THISFILE := $(lastword $(MAKEFILE_LIST))
THISDIR  := $(shell dirname $(realpath $(THISFILE)))
GOBIN    := $(THISDIR)/bin

# Prepend this repo's bin directory to our path since we'll want to
# install some build tools there for use during the build process.
PATH := $(GOBIN):$(PATH)

# Export GOBIN env variable so `go install` picks it up correctly.
export GOBIN

all:

clean:
	$(RM) bin/phenix
	$(RM) tmpl/bindata.go
	$(RM) web/bindata.go
	$(RM) web/proto/*.pb.go
	$(RM) web/public/index.html
	$(RM) web/public/docs/index.html
	$(RM) web/public/favicon.ico
	$(RM) -r web/public/assets
	$(RM) -r web/ui/dist
	$(RM) -r web/ui/node_modules

.PHONY: install-build-deps
install-build-deps: bin/go-bindata bin/mockgen bin/protoc-gen-go

.PHONY: remove-build-deps
remove-build-deps:
	$(RM) bin/go-bindata
	$(RM) bin/mockgen
	$(RM) bin/protoc-gen-go

bin/go-bindata:
	go install github.com/go-bindata/go-bindata/v3/go-bindata

bin/mockgen:
	go install github.com/golang/mock/mockgen

bin/protoc-gen-go:
	go install google.golang.org/protobuf/cmd/protoc-gen-go

.PHONY: generate-bindata
generate-bindata: api/config/bindata.go tmpl/bindata.go web/bindata.go

api/config/bindata.go: $(CONFIGS) bin/go-bindata
	$(GOBIN)/go-bindata -pkg config -prefix api/config/default -o api/config/bindata.go api/config/default/...

tmpl/bindata.go: $(TEMPLATES) bin/go-bindata
	$(GOBIN)/go-bindata -pkg tmpl -prefix tmpl/templates -o tmpl/bindata.go tmpl/templates/...

web/public/index.html: $(UISOURCES)
	(cd web/ui ; yarn install ; yarn build)
	cp -a web/ui/dist/* web/public

web/public/docs/index.html: web/public/docs/openapi.yml
	npx redoc-cli bundle web/public/docs/openapi.yml -o web/public/docs/index.html --title 'phenix API'

web/bindata.go: web/public/index.html web/public/vnc.html web/public/docs/index.html bin/go-bindata
	$(GOBIN)/go-bindata -pkg web -prefix web/public -o web/bindata.go web/public/...

.PHONY: generate-mocks
generate-mocks: app/mock.go internal/mm/mock.go store/mock.go util/shell/mock.go

app/mock.go: app/app.go bin/mockgen
	$(GOBIN)/mockgen -self_package phenix/app -destination app/mock.go -package app phenix/app App

internal/mm/mock.go: internal/mm/mm.go bin/mockgen
	$(GOBIN)/mockgen -self_package phenix/internal/mm -destination internal/mm/mock.go -package mm phenix/internal/mm MM

store/mock.go: store/store.go bin/mockgen
	$(GOBIN)/mockgen -self_package phenix/store -destination store/mock.go -package store phenix/store Store

util/shell/mock.go: util/shell/shell.go bin/mockgen
	$(GOBIN)/mockgen -self_package phenix/util/shell -destination util/shell/mock.go -package shell phenix/util/shell Shell

.PHONY: generate-protobuf
generate-protobuf: web/proto/experiment.pb.go web/proto/host.pb.go web/proto/log.pb.go web/proto/user.pb.go web/proto/vm.pb.go

web/proto/experiment.pb.go: web/proto/*.proto bin/protoc-gen-go
	protoc -I . -I web/proto --go_out=paths=source_relative:. ./web/proto/experiment.proto

web/proto/host.pb.go: web/proto/*.proto bin/protoc-gen-go
	protoc -I . -I web/proto --go_out=paths=source_relative:. ./web/proto/host.proto

web/proto/log.pb.go: web/proto/*.proto bin/protoc-gen-go
	protoc -I . -I web/proto --go_out=paths=source_relative:. ./web/proto/log.proto

web/proto/user.pb.go: web/proto/*.proto bin/protoc-gen-go
	protoc -I . -I web/proto --go_out=paths=source_relative:. ./web/proto/user.proto

web/proto/vm.pb.go: web/proto/*.proto bin/protoc-gen-go
	protoc -I . -I web/proto --go_out=paths=source_relative:. ./web/proto/vm.proto

bin/phenix: $(GOSOURCES) generate-bindata generate-protobuf
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-X 'phenix/version.Version=$(VERSION)' -s -w" -trimpath -o bin/phenix main.go
