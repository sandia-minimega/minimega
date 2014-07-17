// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"math/rand"
	"net/http"
	"time"
)

func easter(w http.ResponseWriter, r *http.Request) {
	s := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(s)
	w.Write([]byte(ronQuotes[rnd.Intn(len(ronQuotes))]))
}

var ronQuotes = []string{
	"You want to make your way in the CS field? Simple. Calculate rough time of amnesia (hell, 10 years is plenty, probably 10 months is plenty), go to the dusty archives, dig out something fun, and go for it. It's worked for many people, and it can work for you.  - Ron Minnich",

	"Tux is not cute.  Tux has the expression of someone right after knitting needles have been used to scramble the front side of their cerebral cortex.  Tux scares me.  - Ron Minnich",

	"Never smack a Bulgarian. - Ron Minnich",

	"All I know about print is that you can do it too soon. - Ron Minnich",

	"Globus is an unstoppable monster - rminnich; No, Globus is an unstartable monster - ericvh.",

	"I guess the idea is that ... even though the structures are all different. Everythings fine, dont worry, lie back and think of	England. ... I think we can call it Pollyanna-morphism. -- Ron Minnich on Python libraries.",

	"As low as possible isn't a number yet - Ron Minnich",
}
