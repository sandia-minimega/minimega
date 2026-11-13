#!/usr/bin/env python3

import json
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import quote

try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib


ROOT_DIR = Path(__file__).resolve().parent.parent
CONFIG_PATH = ROOT_DIR / "zensical.toml"
DOCS_DIR = ROOT_DIR / "doc" / "content"


def format_timestamp(value: str) -> str:
    timestamp = datetime.fromisoformat(value).astimezone(timezone.utc)
    return timestamp.strftime("%Y-%m-%d %H:%M:%S UTC")


def page_url(source: Path, use_directory_urls: bool) -> str:
    relative = source.relative_to(DOCS_DIR)
    is_index = relative.name in {"index.md", "README.md"}
    stem = "index" if relative.name == "README.md" else relative.stem
    if use_directory_urls and not is_index:
        destination = relative.parent / stem / "index.html"
    else:
        destination = relative.parent / f"{stem}.html"

    url = destination.as_posix()
    if use_directory_urls:
        url = url.removesuffix("index.html")
    return quote(url, safe="/")


def git_available() -> bool:
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--is-inside-work-tree"],
            cwd=ROOT_DIR,
            capture_output=True,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        return False
    return result.returncode == 0 and result.stdout.strip() == "true"


def revision_dates(use_directory_urls: bool) -> dict[str, str]:
    fallback = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
    use_git = git_available()
    if not use_git:
        print(
            "warning: Git history unavailable; using build time for page timestamps",
            file=sys.stderr,
        )

    dates = {}
    sources_by_url = {}
    for source in sorted(DOCS_DIR.rglob("*.md")):
        timestamp = ""
        if use_git:
            result = subprocess.run(
                [
                    "git",
                    "log",
                    "-1",
                    "--format=%cI",
                    "--",
                    source.relative_to(ROOT_DIR).as_posix(),
                ],
                cwd=ROOT_DIR,
                capture_output=True,
                text=True,
                check=False,
            )
            if result.returncode != 0:
                raise RuntimeError(result.stderr.strip() or "git log failed")
            timestamp = result.stdout.strip()

        url = page_url(source, use_directory_urls)
        if previous := sources_by_url.get(url):
            raise ValueError(
                f"{previous} and {source} both map to documentation URL {url!r}"
            )
        sources_by_url[url] = source
        dates[url] = (
            format_timestamp(timestamp) if timestamp else fallback
        )
    return dates


def temporary_config() -> Path:
    config = CONFIG_PATH.read_text(encoding="utf-8")
    project = tomllib.loads(config)["project"]
    dates = revision_dates(project.get("use_directory_urls", True))

    with tempfile.NamedTemporaryFile(
        mode="w",
        encoding="utf-8",
        prefix=".zensical-",
        suffix=".toml",
        dir=ROOT_DIR,
        delete=False,
    ) as output:
        output.write(config)
        if not config.endswith("\n"):
            output.write("\n")
        for path, timestamp in dates.items():
            output.write(f"{json.dumps(path)} = {json.dumps(timestamp)}\n")
        return Path(output.name)


def main() -> int:
    if len(sys.argv) < 2 or sys.argv[1] not in {"build", "serve"}:
        print(f"usage: {sys.argv[0]} [build|serve] [options]", file=sys.stderr)
        return 2

    config_path = temporary_config()
    try:
        command = [
            sys.executable,
            "-m",
            "zensical",
            sys.argv[1],
            "--config-file",
            str(config_path),
            *sys.argv[2:],
        ]
        return subprocess.run(command, cwd=ROOT_DIR, check=False).returncode
    finally:
        config_path.unlink(missing_ok=True)


if __name__ == "__main__":
    raise SystemExit(main())
