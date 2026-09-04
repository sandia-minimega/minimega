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

- Follow the [installation guide](articles/installing.md) to build or install
  minimega.
- Use the [quickstart](articles/tutorials/quickstart.md) to launch a first
  virtual experiment.
- Read the [user guide](articles/usage.md) for startup, scripting, cluster, and
  logging details.
- Browse the generated [command API](reference/minimega.md).

## Training

The [Mini101 course](training/module01.md) provides a comprehensive walkthrough
of minimega. Module 1 is an all-in-one guide; later modules cover individual
features in depth. The [historical miniclass](training/miniclass/index.md)
preserves the older self-paced course recovered from the former Sandia website.

## Support and development

- Report bugs and request features in the
  [GitHub issue tracker](https://github.com/sandia-minimega/minimega/issues).
- Read the [contribution guide](articles/contributing.md).
