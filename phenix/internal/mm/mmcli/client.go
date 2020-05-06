// Taken (almost) as-is from minimega/miniweb.

package mmcli

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/activeshadow/libminimega/minicli"
	"github.com/activeshadow/libminimega/miniclient"
)

var (
	mu sync.Mutex
	mm *miniclient.Conn
)

// noop returns a closed channel
func noop() chan *miniclient.Response {
	out := make(chan *miniclient.Response)
	close(out)

	return out
}

func wrapErr(err error) chan *miniclient.Response {
	out := make(chan *miniclient.Response, 1)

	out <- &miniclient.Response{
		Resp: minicli.Responses{
			&minicli.Response{
				Error: err.Error(),
			},
		},
		More: false,
	}

	close(out)

	return out
}

func ErrorResponse(responses chan *miniclient.Response) error {
	for response := range responses {
		for _, resp := range response.Resp {
			if resp.Error != "" {
				return errors.New(resp.Error)
			}
		}
	}

	return nil
}

func SingleResponse(responses chan *miniclient.Response) (string, error) {
	for response := range responses {
		r := response.Resp[0]

		if r.Error != "" {
			return "", errors.New(r.Error)
		}

		return r.Response, nil
	}

	return "", errors.New("no responses")
}

// run minimega commands, automatically redialing if we were disconnected
func Run(c *Command) chan *miniclient.Response {
	mu.Lock()
	defer mu.Unlock()

	var err error

	if mm == nil {
		if mm, err = miniclient.Dial(f_minimegaBase); err != nil {
			return wrapErr(fmt.Errorf("unable to dial: %w", err))
		}
	}

	// check if there's already an error and try to redial
	if err := mm.Error(); err != nil {
		s := err.Error()

		if strings.Contains(s, "broken pipe") || strings.Contains(s, "no such file or directory") {
			if mm, err = miniclient.Dial(f_minimegaBase); err != nil {
				return wrapErr(fmt.Errorf("unable to redial: %w", err))

			}
		} else {
			return wrapErr(fmt.Errorf("minimega error: %w", err))
		}
	}

	return mm.Run(c.String())
}
