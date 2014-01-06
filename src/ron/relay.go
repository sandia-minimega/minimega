// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	log "minilog"
	"net/http"
)

func (r *Ron) newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron", easter)
	http.HandleFunc("/", http.NotFound)

	host := fmt.Sprintf(":%v", r.port)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	return nil
}
