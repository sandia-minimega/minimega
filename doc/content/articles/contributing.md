# Contributing and Development Guide

<a id="TOC_1."></a>

## Contributing code

minimega is an open source project and we welcome contributions from the
community.

<a id="TOC_2."></a>

## Trivial patches

Many patches are simple enough to not need the more rigorous development
approach detailed here. These include typos, additions to documentation without
code revisions, and minor fixes (only a few lines of code). For these, simply
format the patch according to the "Submitting a Patch" section below and issue
a pull request.

When submitting a trivial patch, include the text 'trivial' in the pull request
title.

<a id="TOC_3."></a>

## Discuss your design

New ideas and better solutions to existing code are always welcome. Open a
GitHub issue before implementing a non-trivial change so the design and approach
can be discussed.

<a id="TOC_4."></a>

## Development style

minimega follows the formatting style enforced by the Go language
specification, and *generally* follows the programming style used by core Go
developers (and by luck most Go developers). There are exceptions, particularly
in the few C files in the codebase.

Unless the development in question is aimed at reformatting a piece of the
codebase, you should adhere to the style in that particular section of code.
minimega is growing rapidly and often times newer code is more well thought out
(or not) than older code.

<a id="TOC_5."></a>

## EditorConfig

Use EditorConfig support in your editor so basic formatting rules are applied
automatically when `.editorconfig` files are present in the tree.

If you use VS Code, install the
[EditorConfig for VS Code](https://marketplace.visualstudio.com/items?itemName=EditorConfig.EditorConfig)
extension and ensure it is enabled for your workspace.

Quick verification:

- Open a file in a directory that has an `.editorconfig`.
- Edit whitespace (for example indentation or trailing spaces) and save.
- Confirm the file follows the directory's EditorConfig rules.

EditorConfig complements language-native formatters such as `gofmt`; continue
to run language-specific formatters and tests before opening a pull request.

<a id="TOC_6."></a>

## Tests

Add unit tests using Go's unit test framework whenever possible. If your code
is not Go, tests should be considered in some other capacity.

Always make sure that your code revision builds and completes all tests by
running `all.bash` in the repo. If your code revision applies to windows
binaries (`protonuke`, `miniccc`, etc.), then make sure your change works on
Windows as well.

Write runtime tests using [minitest](minitest.md).

<a id="TOC_7."></a>

## Documentation

Always include updates and additions to the documents, API help, and tutorials
(when appropriate) in your change. Some documentation is currently missing, so
there is an emphasis on ensuring that revisions and especially new
functionality is well documented.

Documentation is built with [Zensical](https://zensical.org/). Build and preview
it locally with:

```bash
python3 -m pip install -r doc/requirements.txt
./scripts/zensical_build.py serve
```

Use `./scripts/zensical_build.py build --strict` to run Zensical's strict link and anchor
validation without starting a preview server. Building generated API
documentation requires Linux and the native minimega build dependencies.

The
[Docs workflow](https://github.com/sandia-minimega/minimega/blob/master/.github/workflows/docs.yml)
validates pull requests and deploys `master` through GitHub Pages. Repository
administrators must select **GitHub Actions** as the Pages source in repository
settings.

<a id="TOC_8."></a>

## Tracking issues

If your change is being committed to a branch other than `master`, include
"updates issue NNN" in the commit log. Never close an issue on a commit in a
branch other than `master` unless that ticket is specific to that branch.

<a id="TOC_9."></a>

## Commit messages

Try to adhere to common Git commit message
[format](https://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).
When working on a one or two tools or packages, we typically prefix the summary
with the its name (e.g. `minimega: ...`, `minimega,minicli: ...`). For commits
that span many components, we typically prefix them with `various: ...`.

<a id="TOC_10."></a>

## Submitting a patch

minimega is hosted on github.com, and we use github's 'pull request'
functionality for code review and merging patches to the main repository. See
[github's pull request page](https://help.github.com/articles/using-pull-requests/)
for more information. In short, you will need to fork minimega to your
own account, commit to your local tree, and issue a pull request to the main
repository from your own.

Generate the pull request from master (or the branch you are committing to) for
all of the commits in your patchset, including updates to documentation.

If your patchset fixes a listed issue, make sure 'fixes issue #NNN' is in the
commit log, so the issue is referenced to your work.

<a id="TOC_11."></a>

## Code review

When a pull request is made, at least one contributor will review the patchset,
make comments, and either wait for more feedback or authorize the patch with a
'LGTM' (looks good to me) phrase. After authorization, any committer may merge
the pull request.

<a id="TOC_12."></a>

## Working with GitHub

See [GitHub's pull request documentation](https://docs.github.com/en/pull-requests)
for more information on effectively contributing to projects through GitHub.

Of specific use may be
[syncing a fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/syncing-a-fork) with an
upstream repo, and
[checking out a pull request locally](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/reviewing-changes-in-pull-requests/checking-out-pull-requests-locally).

<a id="TOC_13."></a>

## Copyright

minimega is released under the GNU GPLv3. Several included 3rd party packages
are released under other licenses, as described in the LICENSES directory in
the repo. If you are submitting a patch to the existing codebase, the code will
be licensed under the same license. If you are submitting an entirely new
package, please license it under any compatible FLOSS license. If you are
submitting under a license not already included in the distribution, discuss it
in a GitHub issue first.

GPLv3 is preferred.
