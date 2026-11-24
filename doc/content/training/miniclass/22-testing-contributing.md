# Chapter 22: Testing and contributing

This last chapter turns the course's own router sandwich into a minitest
case. A minitest case is the kind of functional test that the minimega
repository runs against a live daemon. The chapter then shows what a
contribution to minimega looks like, from the first issue to the release that
ships it. At the end, you have a test file and its expected transcript. You
also know how the suite is run and kept stable, and you know where your first
patch goes.

This chapter assumes the sandwich from [Chapter 3](03-router-sandwich.md) and
the builtins from [Chapter 9](09-save-replay-cleanup.md). The packages have
minitest but not the `tests/` directory. For this reason, this chapter needs a
source checkout built with `./scripts/build.bash` (see
[Installation](../../articles/installing.md#build-from-source)) and the Chapter
2 images. There is no chapter script. The test case is the script from Chapter
3, adapted below.

## How minitest works

A minitest case is a file of minimega commands, without an extension, in
the repository's `tests/` directory. minitest connects to a running
minimega over the command socket and sends each line. It writes a transcript
to `<test>.got`. The transcript has every input line prefixed with `## `,
followed by the rendered output, with errors prefixed by `E: `.

minitest then compares the transcript byte for byte with `<test>.want`. When
they differ, it prints `got != want for <test>`. That is the whole framework.
The `.want` files are the specification of what each command prints.

Two special files in the directory run around every test. `tests/prolog`
holds `.annotate false` and `clear history`. These commands keep host names
out of the transcripts, and each test starts with an empty history.

`tests/epilog` holds `clear all`, `clear plumb`, and `clear pipe`. These
commands kill the VMs and reset the instance without restarting it. Because of
the epilog, tests run in the default namespace and do not need to clean up
after themselves.

## From chapter script to test case

Save the following as `tests/router_sandwich` in your checkout. It is
`03-router-sandwich.mm` with the changes listed below.

```minimega
# The router sandwich from miniclass chapter 3, in the default namespace
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config memory 3072
vm config net net_left
vm launch kvm vm-left
vm config net net_right
vm launch kvm vm-right
vm config kernel $images/minirouter.kernel
vm config initrd $images/minirouter.initrd
vm config net net_left net_right
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

1. The `namespace sandwich` line is gone. The tests of the suite share the
   default namespace. The epilog's `clear all` destroys every namespace, so
   a test does not need to destroy its own.
2. Image paths start with `$images`. minimega expands environment variables
   in arguments. All tests of the suite find their images through this one
   variable. You set the variable in the environment of the daemon, not in
   the test.
3. `.columns name,state,vlan,ip vm info` became `.columns name,state vm
   info`. The `ip` column holds DHCP leases. The order of the leases can
   differ between runs. The `vlan` column holds tags. minimega allocates the
   tags in whatever order it first sees the aliases. A transcript that
   contains these columns does not match on the next run. The `name` and
   `state` columns are what the test is about.
4. The test reads back `router router` before `commit`, so its output is the
   staged configuration and an empty `Log:` section. After the commit, the
   log fills with lines whose contents depend on timing.

minitest echoes comment lines into the transcript. When you read the `.want`
file later, each block then explains itself.

## Run it

Start minimega from the same checkout with `images` in its environment.
Alternatively, set `images` at the prompt with
`.env images /tmp/minimega/files`. This command changes the environment of the
daemon. If `sandwich` is the active namespace of that daemon, run
`clear namespace sandwich` first. This command destroys the VMs of the sandwich
and makes the default namespace active. Then run only this test:

```bash
$ sudo env images=/tmp/minimega/files ./bin/minimega -nostdin &
$ sudo ./bin/minitest -dir tests -run '^router_sandwich$'
```

The first run reports that there is no `router_sandwich.want` yet and
leaves `router_sandwich.got` behind. Read the whole file. It begins:

```text
## # The router sandwich from miniclass chapter 3, in the default namespace
## vm config kernel $images/miniccc.kernel
## vm config initrd $images/miniccc.initrd
## vm config memory 3072
## vm config net net_left
## vm launch kvm vm-left
## vm config net net_right
## vm launch kvm vm-right
## vm config kernel $images/minirouter.kernel
## vm config initrd $images/minirouter.initrd
## vm config net net_left net_right
## vm launch kvm router
## .columns name,state vm info
name     | state
router   | BUILDING
vm-left  | BUILDING
vm-right | BUILDING

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
listing end in spaces, and `E:` lines are sorted. For this reason, you copy
the expectation from a reviewed transcript and do not type it. When every
line is what you meant, accept the transcript and run the test again:

```bash
$ cp tests/router_sandwich.got tests/router_sandwich.want
$ sudo ./bin/minitest -dir tests -run '^router_sandwich$'
```

Silence is a pass. If a later change to minimega alters what one of these
commands prints, the run says `got != want for router_sandwich`. Then
`diff -u tests/router_sandwich.want tests/router_sandwich.got`
shows exactly what changed.

Without `-run`, minitest runs every file in `tests/` (then each
subdirectory). This launches many VMs and needs all the images listed in
[Testing with minitest](../../articles/minitest.md#images). `-base` points
minitest at a daemon that runs with a different base directory. `-level info`
announces each test as it starts. `tests/distributed/` builds a nested
three-node cluster inside VMs to test the mesh and the scheduler. If you have
the images for it, run it separately with `-dir tests/distributed`.

## Keeping tests stable

The suite follows a few rules, and this test followed them:

- Use `.columns` and `.filter` to keep only the columns and rows the test
  is about. IDs, UUIDs, tap names, uptime, leases, and anything timed will
  not match twice.
- Never depend on the VM ID counter, because it survives the epilog.
- Wait with `shell sleep` only as long as a boot or a `cc` round trip
  needs. Then make sure of the result with a command whose output is stable.
- Never accept a `.got` file that you did not read. The `.want` file is the
  claim that your test makes about minimega.

`tests/vm_lifecycle` and `tests/taps_lifecycle` are the models in the
repository for VM and tap tests. `tests/router` covers the whole router
API, including the DHCP and DNS listings above.

## Contributing

minimega is developed on GitHub as
[sandia-minimega/minimega](https://github.com/sandia-minimega/minimega),
and the [contributing guide](../../articles/contributing.md) is the
authority. This is the short version.

Open an issue before any change of more than a few lines, so that the
design can be discussed first. Typos, documentation, and small fixes skip
that step. For these changes, put the word `trivial` in the pull request
title.

A change must bring its evidence with it. Format Go code with `gofmt`, and
follow the style of the file that you change. If a Go unit test is possible,
add one for new behavior. If the behavior is visible at the prompt, add a
minitest case. `./scripts/all.bash` must pass.

If a change touches the patterns or the help text of a command, regenerate
what is derived from them. `./scripts/doc.bash` rebuilds the command
reference pages and `lib/minimega.py`. Put both in the same commit.

Documentation is part of the change, not a follow-up. Before you push, build
it with `./scripts/docs.bash build`. This command fails on
any broken link or anchor.

Commit messages and pull request titles use
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):
`type(scope): subject`, for example `fix(minicli): ...` or
`feat(router): ...`. A commit message or pull request title that closes an
issue also uses `fixes issue #NNN`.

Fork the repository, commit on a branch, and open a pull request against
`master`. A maintainer reviews the pull request and asks for changes or says
`LGTM`. Then any committer can merge it. minimega is GPLv3. New code must use
the same license or a compatible license.

## Releases

The maintainers make releases through GitHub Actions. They coordinate each
release in a pull request that bumps the `VERSION` file. Patch releases need a
passing CI run and an approval. Minor releases add a 48-business-hour window
for objections. Major or breaking releases need two maintainer approvals and a
week.

After the release PR is merged, a maintainer runs the release workflow by
hand. The workflow creates a draft GitHub release with its artifacts. When a
maintainer publishes the draft, a second workflow uploads the Python package to
PyPI. For this reason, you can keep `pip install minimega`
and your daemon at the same version. The
[release process](../../articles/developer/releases.md) has the exact rules,
including when an emergency release can skip the wait.

## What you built

- A minitest case for the router sandwich, with a reviewed `.want`
  transcript. If the commands that it uses change what they print, the case
  fails.
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
