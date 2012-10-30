package minilog

import (
	"testing"
	"bytes"
	"io"
	"strings"
)

func TestMultilog(t *testing.T) {
	sink1 := new(bytes.Buffer)
	sink2 := new(bytes.Buffer)

	AddLogger("sink1", sink1, DEBUG, false)
	AddLogger("sink2", sink2, DEBUG, false)

	test_string := "test 123"

	Debugln(test_string)

	s1 := sink1.String()
	s2 := sink2.String()

	if !strings.Contains(s1,test_string) {
		t.Error("sink1 got:", s1)
	}

	if !strings.Contains(s2,test_string) {
		t.Error("sink2 got:", s2)
	}
}

func TestLogLevels(t *testing.T) {
	sink1 := new(bytes.Buffer)
	sink2 := new(bytes.Buffer)

	AddLogger("sink1", sink1, DEBUG, false)
	AddLogger("sink2", sink2, INFO, false)

	test_string := "test 123"

	Debugln(test_string)

	s1 := sink1.String()
	s2 := sink2.String()

	if !strings.Contains(s1,test_string) {
		t.Error("sink1 got:", s1)
	}

	if len(s2) != 0 {
		t.Error("sink2 got:", s2)
	}
}

func TestDelLogger(t *testing.T) {
	sink := new(bytes.Buffer)

	AddLogger("sink", sink, DEBUG, false)

	test_string := "test 123"
	test_string2 := "test 456"

	Debug(test_string)

	s, err := sink.ReadString('\n')
	if err != nil {
		t.Error(err)
	}

	if !strings.Contains(s,test_string) {
		t.Error("sink got:", s)
	}

	DelLogger("sink")

	Debug(test_string2)

	s,err = sink.ReadString('\n')
	if err != nil && err != io.EOF {
		t.Error(err)
	}

	if len(s) != 0 {
		t.Error("sink got:", s)
	}
}


