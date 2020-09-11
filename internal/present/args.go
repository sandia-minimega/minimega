// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"strconv"
)

func parseArgs(name string, line int, args []string) (res []interface{}, err error) {
	res = make([]interface{}, len(args))
	for i, v := range args {
		if len(v) == 0 {
			return nil, fmt.Errorf("%s:%d bad code argument %q", name, line, v)
		}
		switch v[0] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("%s:%d bad code argument %q", name, line, v)
			}
			res[i] = n
		case '/':
			if len(v) < 2 || v[len(v)-1] != '/' {
				return nil, fmt.Errorf("%s:%d bad code argument %q", name, line, v)
			}
			res[i] = v
		case '$':
			res[i] = "$"
		case '_':
			if len(v) == 1 {
				// Do nothing; "_" indicates an intentionally empty parameter.
				break
			}
			fallthrough
		default:
			return nil, fmt.Errorf("%s:%d bad code argument %q", name, line, v)
		}
	}
	return
}
