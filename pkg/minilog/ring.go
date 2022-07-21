package minilog

import (
	"container/ring"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type Ring struct {
	size int

	// guards below
	mu sync.Mutex
	r  *ring.Ring
}

func NewRing(size int) *Ring {
	return &Ring{
		r:    ring.New(size),
		size: size,
	}
}

// Println mimic golang's log.Logger.Output and prepends the time
func (l *Ring) Println(v ...interface{}) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	var buf []byte

	year, month, day := now.Date()
	buf = strconv.AppendInt(buf, int64(year), 10)
	buf = append(buf, '/')
	buf = strconv.AppendInt(buf, int64(month), 10)
	buf = append(buf, '/')
	buf = strconv.AppendInt(buf, int64(day), 10)
	buf = append(buf, ' ')

	hour, min, sec := now.Clock()
	buf = strconv.AppendInt(buf, int64(hour), 10)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(min), 10)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(sec), 10)
	buf = append(buf, ' ')

	buf = append(buf, fmt.Sprintln(v...)...)

	l.r = l.r.Next()
	l.r.Value = string(buf)
}

// Dump returns the log messages from oldest to newest.
func (l *Ring) Dump() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	res := make([]string, 0, l.size)

	l.r.Next().Do(func(v interface{}) {
		if v == nil {
			return
		}

		res = append(res, v.(string))
	})

	return res
}
