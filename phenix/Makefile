SHELL := /bin/bash

# Default hyperdark version number to the shorthand git commit hash if
# not set at the command line.
VER     := $(or $(VER),$(shell git log -1 --format="%h"))
COMMIT  := $(shell git log -1 --format="%h - %ae")
DATE    := $(shell date -u)
VERSION := $(VER) (commit $(COMMIT)) $(DATE)

GOSOURCES := $(shell find . \( -name '*.go' \))
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
	-rm bin/phenix
	-rm tmpl/bindata.go

.PHONY: install-build-deps
install-build-deps: bin/go-bindata bin/mockgen

.PHONY: remove-build-deps
remove-build-deps:
	$(RM) bin/go-bindata
	$(RM) bin/mockgen

bin/go-bindata:
	go install github.com/go-bindata/go-bindata/v3/go-bindata

bin/mockgen:
	go install github.com/golang/mock/mockgen

.PHONY: generate-bindata
generate-bindata: tmpl/bindata.go

tmpl/bindata.go: $(TEMPLATES) bin/go-bindata
	$(GOBIN)/go-bindata -pkg tmpl -prefix tmpl/templates -o tmpl/bindata.go tmpl/templates/...

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

bin/phenix: $(GOSOURCES) tmpl/bindata.go
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-X 'phenix/version.Version=$(VERSION)' -s -w" -trimpath -o bin/phenix main.go
