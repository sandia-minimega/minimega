SHELL := /bin/bash

SOURCES := $(shell find ./src \( -name '*.js' -o -name '*.vue' \))

all:

clean:
	$(RM) -r dist
	$(RM) -r node_modules

dist/index.html: $(SOURCES)
	(yarn install ; yarn build)