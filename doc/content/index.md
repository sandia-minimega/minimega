# minimega

minimega is a tool for launching and managing virtual machines. It can run on
your laptop or distributed across a cluster. minimega is fast, easy to deploy,
and can scale to run on massive clusters with virtually no setup.

The repository also includes [supporting tools](tools.md) and libraries for
building VM images, managing clusters, and generating traffic.

## How minimega is used

minimega supports repeatable cyber-physical experiments ranging from a single
laptop to large clusters. Sandia uses it to emulate critical infrastructure,
join virtual systems to physical hardware, generate realistic workloads, and
capture experiment data.

<video controls preload="metadata" width="100%"
  poster="assets/legacy-site/video/critical-infrastructure-emulation-and-defense.jpg">
  <source
    src="assets/legacy-site/video/critical-infrastructure-emulation-and-defense.mp4"
    type="video/mp4">
  <a href="assets/legacy-site/video/critical-infrastructure-emulation-and-defense.mp4">
    Download Critical Infrastructure Emulation and Defense
  </a>
</video>

[Watch Critical Infrastructure Emulation and Defense on YouTube](https://www.youtube.com/watch?v=eoO13iV6m_g).

## Getting started

1. [Install minimega](articles/installing.md) from a package, the Docker
   image, or source, and prepare the host.
2. Follow the [quickstart](articles/quickstart.md) to build an image and
   launch a first virtual machine.
3. Learn how to [run minimega](articles/running.md) as a service, with
   startup flags and environment variables.
4. Learn the [command line](articles/cli.md): output filtering, scripts, and
   the command socket.
5. Read [Running in Docker](articles/docker.md) if you use the container
   image.

## User guides

The user guides cover one area each: the
[VM lifecycle](articles/vm-lifecycle.md), the
[virtual machine types](articles/vmtypes.md) and the
[VM configuration reference](articles/vm-config-reference.md),
[building images](articles/vmbetter.md) and
[disk images](articles/disk-images.md),
[host networking](articles/networking.md) and
[routing with minirouter](articles/router.md),
[capture and instrumentation](articles/capture.md),
[command and control](articles/cc.md),
[namespaces](articles/namespaces.md) and
[clusters](articles/cluster.md), and
[troubleshooting](articles/troubleshooting.md). The generated
[command reference](reference/minimega.md) documents every command.

## Training

The [miniclass](training/miniclass/index.md) is the minimega training course.
Part I builds one experiment end to end and is the place to start after the
quickstart; later parts cover each feature area, other guest types, and
clusters. An [instructor syllabus](training/miniclass/syllabus.md) turns it
into a one-day or two-day class.

## Support and development

- Report bugs and request features in the
  [GitHub issue tracker](https://github.com/sandia-minimega/minimega/issues).
- Read the [contribution guide](articles/contributing.md).
- The [historical miniclass](legacy/miniclass/index.md) and other legacy
  material are kept under Legacy for reference.
