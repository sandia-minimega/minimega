package expect

import (
//	"fmt"
	"bufio"
	"regexp"
	"io"
)

type Expecter struct {
	r	io.Reader
	w	io.Writer
	br	*bufio.Reader
	bw	*bufio.Writer
}

func NewExpecter(r io.Reader) *Expecter {
	var ret Expecter
	ret.r = r
	ret.br = bufio.NewReader(r)
	return &ret
}

func (e *Expecter) Expect(s string) (bool, error) {
	return regexp.MatchReader(s, e.br)
}

func (e *Expecter) Send(s string) error {
	_, err := e.bw.WriteString(s)
	if err != nil {
		return err
	}
	err = e.bw.Flush()
	return err
}

func (e *Expecter) SetWriter(w io.Writer) {
	e.bw = bufio.NewWriter(w)
}
