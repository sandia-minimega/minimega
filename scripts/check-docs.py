#!/usr/bin/env python3

import collections
import pathlib
import re
import sys


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


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} DOCS_DIR", file=sys.stderr)
        return 2
    docs_dir = pathlib.Path(sys.argv[1]).resolve()
    references = collections.Counter()
    errors = []

    for doc in docs_dir.rglob("*.md"):
        content = doc.read_text(encoding="utf-8")
        matches = list(EXAMPLE_RE.finditer(content))

        if len(matches) != len(SNIPPET_RE.findall(content)):
            errors.append(f"{doc}: every .mm snippet must have an adjacent download link")
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

    sources = sorted(path.resolve() for path in docs_dir.rglob("*.mm"))
    for source in sources:
        count = references[source]
        if count != 1:
            errors.append(
                f"{source.relative_to(docs_dir)}: expected one documented use, found {count}"
            )

    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1

    print(f"validated {len(sources)} downloadable minimega examples")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
