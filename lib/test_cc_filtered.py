"""Tests for minimega.cc_filtered (python3 -m unittest discover -s lib -p 'test_*.py').

Needs lib/minimega.py, which scripts/build.bash generates with pyapigen.
"""
import unittest

import minimega


class Recorder(minimega.minimega):
    def __init__(self, filter_resp=None, filter_raises=False):
        self.calls = []
        self.moreResponses = False
        self._filter_resp = filter_resp or [{"Error": ""}]
        self._filter_raises = filter_raises

    def cc_filter(self, f):
        self.calls.append(("filter", f))
        if self._filter_raises:
            raise minimega.Error("bad filter on another host")
        return self._filter_resp

    def clear_cc_filter(self):
        if self.moreResponses:
            raise minimega.Error("more responses to be read from last command")
        self.calls.append(("clear",))
        return [{"Error": ""}]

    def streamResponses(self):
        self.moreResponses = False
        self.calls.append(("drain",))
        yield [{"Error": "stream failed"}]
        raise minimega.Error("stream failed")


class TestCCFiltered(unittest.TestCase):
    def test_clears_on_exit(self):
        mm = Recorder()
        with mm.cc_filtered("uuid=a", "os=linux"):
            mm.calls.append(("body",))
        self.assertEqual(mm.calls, [
            ("filter", "uuid=a os=linux"), ("body",), ("clear",),
        ])

    def test_clears_and_keeps_block_error(self):
        mm = Recorder()
        with self.assertRaises(RuntimeError):
            with mm.cc_filtered("uuid=b"):
                raise RuntimeError
        self.assertEqual(mm.calls, [("filter", "uuid=b"), ("clear",)])

    def test_rejects_blank_filter(self):
        for filters in [(), ("",), (" ", "")]:
            mm = Recorder()
            with self.assertRaises(ValueError):
                with mm.cc_filtered(*filters):
                    self.fail("block ran without a filter")
            self.assertEqual(mm.calls, [])

    def test_clears_when_setup_raises(self):
        mm = Recorder(filter_raises=True)
        with self.assertRaises(minimega.Error):
            with mm.cc_filtered("uuid=c"):
                self.fail("block ran after the filter failed")
        self.assertEqual(mm.calls, [("filter", "uuid=c"), ("clear",)])

    def test_raises_on_error_response(self):
        # raise_errors=False: the error comes back in the response
        mm = Recorder(filter_resp=[{"Error": ""}, {"Error": "malformed"}])
        with self.assertRaisesRegex(minimega.Error, "malformed"):
            with mm.cc_filtered("uuid=d"):
                self.fail("block ran after the filter failed")
        self.assertEqual(mm.calls, [("filter", "uuid=d"), ("clear",)])

    def test_drains_pending_responses(self):
        mm = Recorder()
        with self.assertRaises(RuntimeError):
            with mm.cc_filtered("uuid=e"):
                mm.moreResponses = True
                raise RuntimeError
        self.assertEqual(mm.calls, [("filter", "uuid=e"), ("drain",), ("clear",)])


if __name__ == "__main__":
    unittest.main()
