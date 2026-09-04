import re

from pygments.lexer import RegexLexer, words
from pygments.token import (
    Comment,
    Keyword,
    Name,
    Number,
    Operator,
    String,
    Text,
    Whitespace,
)


class MinimegaLexer(RegexLexer):
    """Highlight minimega command files."""

    name = "minimega"
    aliases = ["minimega", "mm"]
    filenames = ["*.mm"]
    url = "https://sandia-minimega.github.io/minimega/"
    flags = re.MULTILINE

    tokens = {
        "root": [
            (r"\s+", Whitespace),
            (r"#.*$", Comment.Single),
            (r'"(?:\\.|[^"\\])*"', String.Double),
            (r"'(?:\\.|[^'\\])*'", String.Single),
            (r"\$\{[^}]+\}|\$[A-Za-z_][A-Za-z0-9_]*", Name.Variable),
            (
                words(
                    (
                        ".alias",
                        ".annotate",
                        ".column",
                        ".columns",
                        ".compress",
                        ".csv",
                        ".env",
                        ".filter",
                        ".headers",
                        ".json",
                        ".preprocess",
                        ".record",
                        ".sort",
                    ),
                    suffix=r"\b",
                ),
                Name.Builtin,
            ),
            (
                words(
                    (
                        "bridge",
                        "capture",
                        "cc",
                        "check",
                        "clear",
                        "deploy",
                        "debug",
                        "disk",
                        "dnsmasq",
                        "file",
                        "help",
                        "history",
                        "host",
                        "log",
                        "mesh",
                        "namespace",
                        "ns",
                        "optimize",
                        "plumbing",
                        "qos",
                        "quit",
                        "read",
                        "router",
                        "shell",
                        "tap",
                        "version",
                        "vnc",
                        "vlan",
                        "vlans",
                        "vm",
                        "write",
                    ),
                    prefix=r"\b",
                    suffix=r"\b",
                ),
                Keyword,
            ),
            (
                words(
                    (
                        "add",
                        "config",
                        "delete",
                        "destroy",
                        "flush",
                        "info",
                        "kill",
                        "launch",
                        "list",
                        "pause",
                        "resume",
                        "show",
                        "start",
                        "status",
                        "stop",
                    ),
                    prefix=r"\b",
                    suffix=r"\b",
                ),
                Name.Function,
            ),
            (r"\b(?:true|false|none)\b", Keyword.Constant),
            (r"\b(?:\d{1,3}\.){3}\d{1,3}(?:/\d{1,2})?\b", Number),
            (r"\b0x[0-9A-Fa-f]+\b|\b\d+(?:\.\d+)?\b", Number),
            (r"[=|,:/]+", Operator),
            (r"[^\s#\"'$=|,:/]+", Text),
            (r".", Text),
        ],
    }
