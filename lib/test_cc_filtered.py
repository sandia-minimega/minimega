"""Runnable check for minimega.cc_filtered (python3 -m pytest lib, or run directly).

Needs lib/minimega.py, which scripts/build.bash generates with pyapigen.
"""
import minimega


class Recorder(minimega.minimega):
    def __init__(self):
        self.calls = []

    def cc_filter(self, f):
        self.calls.append(("filter", f))

    def clear_cc_filter(self):
        self.calls.append(("clear",))


def test_clears_on_exit_and_on_error():
    mm = Recorder()
    with mm.cc_filtered("uuid=a", "os=linux"):
        mm.calls.append(("body",))
    try:
        with mm.cc_filtered("uuid=b"):
            raise RuntimeError
    except RuntimeError:
        pass
    assert mm.calls == [
        ("filter", "uuid=a os=linux"), ("body",), ("clear",),
        ("filter", "uuid=b"), ("clear",),
    ]


if __name__ == "__main__":
    test_clears_on_exit_and_on_error()
