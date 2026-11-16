// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minilog

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFilter(t *testing.T) {
	sink1 := new(bytes.Buffer)

	AddLogger("sink1Level", sink1, DEBUG, false)

	testString := "test 123"
	testString2 := "test 456"

	Debugln(testString)

	s1 := sink1.String()

	if !strings.Contains(s1, testString) {
		t.Fatal("sink1 got:", s1)
	}

	AddFilter("sink1Level", "minilog_test")

	Debugln(testString2)

	s1 = sink1.String()

	if strings.Contains(s1, testString2) {
		t.Fatal("sink1 got:", s1)
	}

	DelFilter("sink1Level", "minilog_test")

	Debugln(testString2)

	s1 = sink1.String()

	if !strings.Contains(s1, testString2) {
		t.Fatal("sink1 got:", s1)
	}
}

func TestMultilog(t *testing.T) {
	sink1 := new(bytes.Buffer)
	sink2 := new(bytes.Buffer)

	AddLogger("sink1", sink1, DEBUG, false)
	AddLogger("sink2", sink2, DEBUG, false)

	testString := "test 123"

	Debugln(testString)

	s1 := sink1.String()
	s2 := sink2.String()

	if !strings.Contains(s1, testString) {
		t.Fatal("sink1 got:", s1)
	}

	if !strings.Contains(s2, testString) {
		t.Fatal("sink2 got:", s2)
	}
}

func TestLogLevels(t *testing.T) {
	sink1 := new(bytes.Buffer)
	sink2 := new(bytes.Buffer)

	AddLogger("sink1Level", sink1, DEBUG, false)
	AddLogger("sink2Level", sink2, INFO, false)

	testString := "test 123"

	Debugln(testString)

	s1 := sink1.String()
	s2 := sink2.String()

	if !strings.Contains(s1, testString) {
		t.Fatal("sink1 got:", s1)
	}

	if len(s2) != 0 {
		t.Fatal("sink2 got:", s2)
	}
}

func TestDelLogger(t *testing.T) {
	sink := new(bytes.Buffer)

	AddLogger("sinkDel", sink, DEBUG, false)

	testString := "test 123"
	testString2 := "test 456"

	Debug("%s", testString)

	s, err := sink.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(s, testString) {
		t.Fatal("sink got:", s)
	}

	DelLogger("sinkDel")

	Debug("%s", testString2)

	s, err = sink.ReadString('\n')
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	if len(s) != 0 {
		t.Fatal("sink got:", s)
	}
}

// lockedBuffer is a bytes.Buffer that is safe to write from the LogAll
// goroutine while the test reads it.
type lockedBuffer struct {
	sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.Lock()
	defer b.Unlock()

	return b.buf.String()
}

func TestLogAll(t *testing.T) {
	out := new(lockedBuffer)
	source := bytes.NewBufferString("line_1\nline_2\nline_3")

	AddLogger("sinkAll", out, DEBUG, false)
	defer DelLogger("sinkAll")

	LogAll(source, DEBUG, "test")

	// wait for the LogAll goroutine to log all three lines
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(out.String(), "\n") < 3 {
		if time.Now().After(deadline) {
			t.Fatal("sink got:", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	sink := bytes.NewBufferString(out.String())

	// we should see only three lines on the logger output
	l1, err := sink.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l1, "line_1") {
		t.Fatal("sink got:", l1)
	}

	l2, err := sink.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l2, "line_2") {
		t.Fatal("sink got:", l2)
	}

	l3, err := sink.ReadString('\n')
	if err != nil {
		t.Fatal(err, l3)
	}
	if !strings.Contains(l3, "line_3") {
		t.Fatal("sink got:", l3)
	}

	oops, err := sink.ReadString('\n')
	if err != io.EOF {
		t.Fatal(err, oops)
	}
}

func BenchmarkLogging(b *testing.B) {
	null, err := os.Create(os.DevNull)
	if err != nil {
		b.Fatal(err)
	}

	AddLogger("null", null, DEBUG, false)
	defer DelLogger("null")

	// waitgroup for logging completed
	var wg sync.WaitGroup

	// Create a bunch of goroutines, firing off b.N messages each
	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			for j := 0; j < b.N; j++ {
				log(DEBUG, "", "message from %v: %v/%v", i, j, b.N)
			}
		}(i)
	}

	wg.Wait()
}
