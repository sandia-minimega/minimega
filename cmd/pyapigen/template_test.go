// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonTemplateCCFiltered(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}

	dir := t.TempDir()
	modulePath := filepath.Join(dir, "minimega.py")
	module, err := os.Create(modulePath)
	if err != nil {
		t.Fatal(err)
	}

	data := struct {
		Commands map[string]*Command
		Version  string
		Date     string
	}{
		Commands: map[string]*Command{
			"cc_filter": {
				Name: "cc_filter",
				Help: "filter clients\n\tcc filter tag=foo:bar",
				Args: []Arg{{Name: "filter", Optional: true}},
				Variants: []*Variant{{
					Pattern:      "cc filter [filter]",
					TemplateArgs: []string{"\"cc\"", "\"filter\"", "filter"},
					Args:         []Arg{{Name: "filter", Optional: true}},
				}},
			},
		},
		Version: "test",
		Date:    "test",
	}
	if err := Template.Execute(module, data); err != nil {
		t.Fatal(err)
	}
	if err := module.Close(); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "\t") {
		t.Fatal("generated Python contains tabs")
	}

	testPath := filepath.Join(dir, "test_cc_filtered.py")
	if err := os.WriteFile(testPath, []byte(ccFilteredTestScript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(python, testPath)
	cmd.Env = append(os.Environ(), "PYTHONPATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cc_filtered Python test failed: %v\n%s", err, out)
	}
}

const ccFilteredTestScript = `
import minimega


class Recorder(minimega.minimega):
    def __init__(self):
        self.calls = []
        self.moreResponses = False
        self.filter_error = ''
        self.clear_error = None
        self.stream_error = None
        self.stream_error_response = ''
        self._raise_errors = True

    def _get_response(self):
        raise AssertionError('not used')

    def cc_filter(self, f):
        self.calls.append(('filter', f))
        if self._raise_errors and self.filter_error:
            raise minimega.Error(self.filter_error)
        return [{'Error': self.filter_error}]

    def clear_cc_filter(self):
        self.calls.append(('clear',))
        if self.clear_error is not None:
            raise self.clear_error
        if self._raise_errors:
            raise AssertionError('clear_cc_filter called with raise_errors enabled')
        return [{'Error': ''}]

    def streamResponses(self):
        self.calls.append(('stream',))
        self.moreResponses = False
        if self.stream_error is not None:
            raise self.stream_error
        yield [{'Error': self.stream_error_response}]


def test_namespace_context_manager_still_shallow_copies():
    mm = Recorder()
    mm._namespace = 'outer'
    with mm.namespace('inner') as ns:
        assert ns is not mm
        assert ns._namespace == 'inner'
        assert mm._namespace == 'outer'
        assert ns.calls is mm.calls

    try:
        mm.__reduce__()
    except TypeError:
        pass
    else:
        raise AssertionError('pickle reduction was not rejected')


def test_clears_on_exit_and_on_error():
    mm = Recorder()
    with mm.cc_filtered('uuid=a', 'os=linux'):
        mm.calls.append(('body',))
    try:
        with mm.cc_filtered('uuid=b'):
            raise RuntimeError('body')
    except RuntimeError as e:
        assert str(e) == 'body'
    else:
        raise AssertionError('body exception was not raised')
    assert mm.calls == [
        ('filter', 'uuid=a os=linux'), ('body',), ('clear',),
        ('filter', 'uuid=b'), ('clear',),
    ]


def test_rejects_blank_filter():
    for args in [(), ('',), (' ', '')]:
        mm = Recorder()
        try:
            with mm.cc_filtered(*args):
                raise AssertionError('entered block')
        except ValueError:
            pass
        else:
            raise AssertionError('blank filter was accepted')
        assert mm.calls == []


def test_setup_error_raises_before_yield_and_clears():
    for raise_errors in [True, False]:
        mm = Recorder()
        mm._raise_errors = raise_errors
        mm.filter_error = 'bad filter'
        try:
            with mm.cc_filtered('bad'):
                raise AssertionError('entered block')
        except minimega.Error as e:
            assert str(e) == 'bad filter'
        else:
            raise AssertionError('filter error was not raised')
        assert mm.calls == [('filter', 'bad'), ('clear',)]


def test_setup_stream_error_raises_before_yield_and_clears():
    mm = Recorder()
    mm.moreResponses = True
    mm.stream_error_response = 'remote setup failed'
    try:
        with mm.cc_filtered('uuid=a'):
            raise AssertionError('entered block')
    except minimega.Error as e:
        assert str(e) == 'remote setup failed'
    else:
        raise AssertionError('streamed setup error was not raised')
    assert mm.calls == [('filter', 'uuid=a'), ('stream',), ('clear',)]


def test_setup_exception_still_clears():
    class FailingFilter(Recorder):
        def cc_filter(self, f):
            self.calls.append(('filter', f))
            raise minimega.Error('remote setup failed')

    mm = FailingFilter()
    try:
        with mm.cc_filtered('uuid=a'):
            raise AssertionError('entered block')
    except minimega.Error as e:
        assert str(e) == 'remote setup failed'
    else:
        raise AssertionError('setup exception was not raised')
    assert mm.calls == [('filter', 'uuid=a'), ('clear',)]


def test_drains_pending_responses_before_clear():
    mm = Recorder()
    with mm.cc_filtered('uuid=a'):
        mm.moreResponses = True
    assert mm.calls == [('filter', 'uuid=a'), ('stream',), ('clear',)]


def test_body_exception_survives_stream_and_clear_errors():
    mm = Recorder()
    mm.stream_error = minimega.Error('stream failed')
    mm.clear_error = minimega.Error('clear failed')
    try:
        with mm.cc_filtered('uuid=a'):
            mm.moreResponses = True
            raise RuntimeError('body failed')
    except RuntimeError as e:
        assert str(e) == 'body failed'
    else:
        raise AssertionError('body exception was not raised')
    assert mm.calls == [('filter', 'uuid=a'), ('stream',), ('clear',)]


def test_cleanup_error_raises_without_body_exception():
    mm = Recorder()
    mm.stream_error = minimega.Error('stream failed')
    try:
        with mm.cc_filtered('uuid=a'):
            mm.moreResponses = True
    except minimega.Error as e:
        assert str(e) == 'stream failed'
    else:
        raise AssertionError('cleanup exception was not raised')
    assert mm.calls == [('filter', 'uuid=a'), ('stream',), ('clear',)]


if __name__ == '__main__':
    test_namespace_context_manager_still_shallow_copies()
    test_clears_on_exit_and_on_error()
    test_rejects_blank_filter()
    test_setup_error_raises_before_yield_and_clears()
    test_setup_stream_error_raises_before_yield_and_clears()
    test_setup_exception_still_clears()
    test_drains_pending_responses_before_clear()
    test_body_exception_survives_stream_and_clear_errors()
    test_cleanup_error_raises_without_body_exception()
`
