// Taken (almost) as-is from minimega/miniweb.

package mmcli

import (
	"errors"
	"fmt"
	"phenix/internal/common"
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

// ErrorResponse is used when only concerned with errors returned from a call to
// minimega. The first error encountered will be returned.
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

// SingleReponse is used when only a single response (or error) is expected to
// be returned from a call to minimega.
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

// Run dials the minimega Unix socket and runs the given command, automatically
// redialing if disconnected. Any errors encountered will be returned as part of
// the response channel.
func Run(c *Command) chan *miniclient.Response {
	mu.Lock()
	defer mu.Unlock()

	var err error

	if mm == nil {
		if mm, err = miniclient.Dial(common.MinimegaBase); err != nil {
			return wrapErr(fmt.Errorf("unable to dial: %w", err))
		}
	}

	// check if there's already an error and try to redial
	if err := mm.Error(); err != nil {
		s := err.Error()

		if strings.Contains(s, "broken pipe") || strings.Contains(s, "no such file or directory") {
			if mm, err = miniclient.Dial(common.MinimegaBase); err != nil {
				return wrapErr(fmt.Errorf("unable to redial: %w", err))

			}
		} else {
			return wrapErr(fmt.Errorf("minimega error: %w", err))
		}
	}

	return mm.Run(c.String())
}
