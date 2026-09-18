# Chapter 22: Testing and contributing

This last chapter turns the course's own router sandwich into a minitest
case, the kind of functional test the minimega repository runs against a
live daemon, and then shows what a contribution to minimega looks like from
the first issue to the release that ships it. At the end you have a test
file and its expected transcript, know how the suite is run and kept
stable, and know where your first patch goes.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md) and the
builtins from [Chapter 9](09-save-replay-cleanup.md). minitest is not in
the packages, so this chapter needs a source checkout built with
`./scripts/build.bash` (see
[Installation](../../articles/installing.md#build-from-source)), and the
Chapter 2 images. There is no chapter script: the test case *is* the
script from Chapter 3, adapted below.

## How minitest works

A minitest case is a file of minimega commands, without an extension, in
the repository's `tests/` directory. minitest connects to a running
minimega over the command socket, sends each line, and writes a transcript
to `<test>.got`: every input line prefixed with `## `, followed by the
rendered output, with errors prefixed by `E: `. It then compares the
transcript byte for byte with `<test>.want` and prints
`got != want for <test>` when they differ. That is the whole framework; the
`.want` files are the specification of what each command prints.

Two special files in the directory run around every test. `tests/prolog`
holds `.annotate false` and `clear history`, so host names stay out of the
transcripts and each test starts with an empty history; `tests/epilog`
holds `clear all`, `clear plumb`, and `clear pipe`, which kills the VMs and
resets the instance without restarting it. Because of the epilog, tests run
in the default namespace and do not need to clean up after themselves.

## From chapter script to test case

Save the following as `tests/router_sandwich` in your checkout. It is
`03-router-sandwich.mm` with the changes listed below.

```minimega
# The router sandwich from miniclass chapter 3, in the default namespace
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right
vm config kernel $images/minirouter.kernel
vm config initrd $images/minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
.columns name,state vm info

# Stage the router configuration and read it back before committing
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router
router router commit

# Boot and check that everything reaches RUNNING
vm start router
shell sleep 10
vm start all
shell sleep 20
.columns name,state vm info
```

1. The `namespace sandwich` line is gone. The suite's tests share the
   default namespace, which the epilog's `clear all` resets; a test that
   creates a namespace would have to destroy it itself, and the next test
   would otherwise inherit its VMs.
2. Image paths start with `$images`. minimega expands environment variables
   in arguments, and the suite's tests all find their images through this
   one variable, which you set in the daemon's environment rather than in
   the test.
3. `.columns name,state,vlan,ip vm info` became `.columns name,state vm
   info`. The `ip` column holds DHCP leases, which are not guaranteed to
   come out in the same order twice, and `vlan` holds tags allocated in
   whatever order the aliases were first seen. A transcript that contains
   them will not match on the next run. The `name` and `state` columns
   are what the test is about.
4. `router router` is read back *before* `commit`, so its output is the
   staged configuration and an empty `Log:` section. After the commit the
   log fills with lines whose contents depend on timing.

Comment lines are echoed into the transcript, so each block explains
itself when you read the `.want` file later.

## Run it

Start minimega from the same checkout with `images` in its environment (or
set it at the prompt with `.env images /tmp/minimega/files`, which changes
the daemon's environment), then run just this test:

```bash
$ sudo env images=/tmp/minimega/files ./bin/minimega -nostdin &
$ sudo ./bin/minitest -dir tests -run '^router_sandwich$'
```

The first run complains that there is no `router_sandwich.want` yet and
leaves `router_sandwich.got` behind. Read it, all of it. It begins:

```text
## # The router sandwich from miniclass chapter 3, in the default namespace
## vm config kernel $images/miniccc.kernel
## vm config initrd $images/miniccc.initrd
## vm config memory 2048
## vm config networks net_left
## vm launch kvm vm_left
## vm config networks net_right
## vm launch kvm vm_right
## vm config kernel $images/minirouter.kernel
## vm config initrd $images/minirouter.initrd
## vm config networks net_left net_right
## vm launch kvm router
## .columns name,state vm info
name     | state
router   | BUILDING
vm_left  | BUILDING
vm_right | BUILDING

## # Stage the router configuration and read it back before committing
## router router interface 0 10.0.0.1/24
...
## router router
IPs:
Network: 0: [10.0.0.1/24]
Network: 1: [10.0.1.1/24]

Listen address: 10.0.0.0
Low address:    10.0.0.2
High address:   10.0.0.254
Router:         10.0.0.1
...
```

and ends with the same three VMs in `RUNNING`. Some lines of the router
listing end in spaces, and `E:` lines are sorted; this is why the
expectation is copied from a reviewed transcript rather than typed. When
every line is what you meant, accept it and run again:

```bash
$ cp tests/router_sandwich.got tests/router_sandwich.want
$ sudo ./bin/minitest -dir tests -run '^router_sandwich$'
```

Silence is a pass. If a later change to minimega alters what one of these
commands prints, the run says `got != want for router_sandwich` and
`diff -u tests/router_sandwich.want tests/router_sandwich.got` shows
exactly what moved.

Without `-run`, minitest runs every file in `tests/` (then each
subdirectory), which launches many VMs and needs all the images listed in
[Testing with minitest](../../articles/minitest.md#images); `-base` points
it at a daemon running with a different base directory, and `-level info`
announces each test as it starts. `tests/distributed/` builds a nested
three-node cluster inside VMs to exercise the mesh and scheduler; run it
separately with `-dir tests/distributed` when you have the images for it.

## Keeping tests stable

The rules the suite follows, and that this test followed, are few:

- Use `.columns` and `.filter` to keep only the columns and rows the test
  is about. IDs, UUIDs, tap names, uptime, leases, and anything timed will
  not match twice.
- Never depend on the VM ID counter: it survives the epilog.
- Wait with `shell sleep` only as long as a boot or a `cc` round trip
  needs, and check the result with a command whose output is stable.
- Never accept a `.got` you have not read. The `.want` file is the claim
  your test makes about minimega.

`tests/vm_lifecycle` and `tests/taps_lifecycle` are the models in the
repository for VM and tap tests; `tests/router` covers the whole router
API, including the DHCP and DNS listings above.

## Contributing

minimega is developed on GitHub as
[sandia-minimega/minimega](https://github.com/sandia-minimega/minimega),
and the [contributing guide](../../articles/contributing.md) is the
authority; this is the short version.

Open an issue before any change that is more than a few lines, so the
design can be discussed first. Typos, documentation, and small fixes skip
that step: put the word `trivial` in the pull request title.

A change is expected to bring its evidence with it. Go code is formatted
with `gofmt` and follows the style of the file it lands in; new behaviour
gets a Go unit test where one is possible and a minitest case where the
behaviour is visible at the prompt, and `./scripts/all.bash` must pass.
Anything that touches a command's patterns or help text also regenerates
what is derived from them: `./scripts/doc.bash` rebuilds the command
reference pages and `lib/minimega.py`, and both belong in the same commit.
Documentation is part of the change, not a follow-up; build it with
`./scripts/zensical_build.py build --strict`, which fails on any broken
link or anchor, before you push.

Commit messages carry the component as a prefix (`minimega: ...`,
`minicli,minimega: ...`, `various: ...` for a change across many), and
`fixes issue #NNN` when they close an issue. Fork the repository, commit on
a branch, and open a pull request against `master`; a maintainer reviews
it, asks for changes or says `LGTM`, and any committer may then merge it.
minimega is GPLv3, and new code is expected under the same or a compatible
license.

## Releases

Releases are made by the maintainers through GitHub Actions and are
coordinated in a pull request that bumps the `VERSION` file. Patch
releases need a passing CI run and an approval; minor releases add a
48-business-hour window for objections; major or breaking releases need
two maintainer approvals and a week. Once the release PR is merged, a
maintainer runs the release workflow by hand, which creates the GitHub
release, its artifacts, and the matching Python package on PyPI, which is
why `pip install minimega` and your daemon can be kept at the same version.
The [release process](../../articles/developer/releases.md) has the exact
rules, including when an emergency release may skip the wait.

## What you built

- A minitest case for the router sandwich, with a reviewed `.want`
  transcript, that fails if the commands it uses ever change what they
  print.
- The habits that keep a transcript test stable across runs and hosts.
- The route for a contribution: issue, tests, docs, regenerated reference
  and bindings, pull request, review, release.

## Where to read more

- [Testing with minitest](../../articles/minitest.md): the runner's flags,
  the special files, the images the suite needs, and the distributed tests.
- [Contributing](../../articles/contributing.md)
- [Releases](../../articles/developer/releases.md)
- [Python API](../../articles/python.md#regenerating-the-bindings) for what
  `./scripts/doc.bash` regenerates.
- [Command line and scripting](../../articles/cli.md#output-builtins) for
  `.columns`, `.filter`, `.annotate`, and `.env`.
