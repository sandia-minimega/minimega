# phenix ![status](https://img.shields.io/badge/status-alpha-red.svg) ![Docker Cloud Automated build](https://img.shields.io/docker/cloud/automated/activeshadow/phenix) ![Docker Cloud Build Status](https://img.shields.io/docker/cloud/build/activeshadow/phenix)

Welcome to `phenix`!

## Building

To build locally, you will need Golang v1.14 and Node v14.2 installed. Once
installed (if not already), simply run `make bin/phenix`.

If you don't want to install Golang and/or Node locally, you can also use
Docker to build phenix (assuming you have Docker installed). Simply run
`./build-with-docker.sh` and once built, the phenix binary will be available
at `bin/phenix`. See `./build-with-docker.sh -h` for usage details.

A Docker image is also hosted on Docker Hub and can be pulled via:

```
$> docker pull activeshadow/phenix
```

Right now there's only a single `latest` tag used for the image on Docker
Hub, and the image is updated automatically each time a commit is pushed to
the `phenix` branch.

> **NOTE**: currently the `latest` image available on Docker Hub defaults to
> having UI authentication disabled. If you want to enable authentication,
> you'll need to build the image yourself, setting the `PHENIX_WEB_AUTH=enabled`
> Docker build argument. See issue #29 for additional details.

## Using

Please see the documentation at https://activeshadow.github.io/minimega for
phenix usage documentation.
