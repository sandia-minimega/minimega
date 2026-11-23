#!/usr/bin/env python3

import collections
import pathlib
import re
import sys
import urllib.parse


EXAMPLE_RE = re.compile(
    r'^```minimega title="(?P<title>[^"]+\.mm)"\n'
    r'--8<-- "(?P<source>[^"]+\.mm)"\n'
    r"```\n\n"
    r'\[Download this example\]\((?P<link>[^)]+)\)'
    r'\{ download="(?P<download>[^"]+)" \}$',
    re.MULTILINE,
)
SNIPPET_RE = re.compile(r'^--8<-- "[^"]+\.mm"$', re.MULTILINE)
DOWNLOAD_RE = re.compile(r"^\[Download this example\]", re.MULTILINE)
# lychee.toml excludes these links, so this script checks them.
REPO_LINK_RE = re.compile(
    r"(?P<open>[(<]?)https://github\.com/sandia-minimega/minimega/(?:blob|tree)/master/"
    r"(?P<path>[^\s)<>\"'`#?]+)"
)
REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent
# The lychee inputs in .github/workflows/docs.yml that are not in DOCS_DIR.
LINK_CHECK_FILES = ("README.md", "docker/README.md")


def check_repo_links(doc: pathlib.Path, content: str) -> tuple[int, list[str]]:
    errors = []
    links = list(REPO_LINK_RE.finditer(content))
    for link in links:
        path = link["path"]
        # A bare URL does not include trailing punctuation, as in a GitHub autolink.
        if not link["open"]:
            path = path.rstrip(".,;:!*_~")
        target = (REPO_ROOT / urllib.parse.unquote(path)).resolve()
        if not target.is_relative_to(REPO_ROOT) or not target.exists():
            errors.append(f"{doc}: link to missing repository file {path}")
    return len(links), errors


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} DOCS_DIR", file=sys.stderr)
        return 2
    docs_dir = pathlib.Path(sys.argv[1]).resolve()
    references = collections.Counter()
    errors = []
    repo_links = 0

    for doc in docs_dir.rglob("*.md"):
        content = doc.read_text(encoding="utf-8")
        matches = list(EXAMPLE_RE.finditer(content))

        count, link_errors = check_repo_links(doc, content)
        repo_links += count
        errors.extend(link_errors)

        if len(matches) != len(SNIPPET_RE.findall(content)):
            errors.append(
                f"{doc}: every .mm snippet must have an adjacent download link"
            )
        if len(matches) != len(DOWNLOAD_RE.findall(content)):
            errors.append(f"{doc}: unexpected or missing example download link")

        for match in matches:
            source = (docs_dir / match["source"]).resolve()
            link = (doc.parent / match["link"]).resolve()
            basename = source.name

            try:
                source.relative_to(docs_dir)
            except ValueError:
                errors.append(f"{doc}: snippet source escapes documentation directory")
                continue

            if not source.is_file():
                errors.append(f"{doc}: missing snippet source {match['source']}")
                continue
            if link != source:
                errors.append(f"{doc}: download link does not target {match['source']}")
            if match["title"] != basename:
                errors.append(f"{doc}: snippet title must be {basename}")
            if match["download"] != basename:
                errors.append(f"{doc}: download attribute must be {basename}")

            references[source] += 1

    for name in LINK_CHECK_FILES:
        doc = REPO_ROOT / name
        count, link_errors = check_repo_links(doc, doc.read_text(encoding="utf-8"))
        repo_links += count
        errors.extend(link_errors)

    sources = sorted(path.resolve() for path in docs_dir.rglob("*.mm"))
    for source in sources:
        count = references[source]
        if count != 1:
            errors.append(
                f"{source.relative_to(docs_dir)}: "
                f"expected one documented use, found {count}"
            )

    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1

    print(f"validated {len(sources)} downloadable minimega examples")
    print(f"validated {repo_links} links to repository files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
