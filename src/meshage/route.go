package meshage

import (
	log "minilog"
)

// find and record the next hop route for client. 
// Additionally, all hops along this route are also the shortest path, so record those as well to 
// save on effort.
func (n *Node) updateRoute(client string) {
	if len(n.mesh) == 0 {
		return
	}

	log.Debugln("updating route for ", client)

	routes := make(map[string]string) // a key node has a value of the previous hop, the key exists if it's been visited
	routes[n.name] = n.name

	q := make(chan string, len(n.mesh))
	q <- n.name

	for len(q) != 0 {
		v := <-q

		log.Debug("visiting %v\n", v)

		for _, a := range n.mesh[v] {
			if _, ok := routes[a]; !ok {
				q <- a
				log.Debug("previous hop for %v is %v\n", a, v)
				routes[a] = v
			}
		}

		if v == client {
			break
		}
	}

	// parse the routes, find the first hop, and add them to the routing table
	for k, v := range routes {
		curr := v
		prev := k
		r := k
		for curr != n.name {
			prev = curr
			curr = routes[curr]
			r += "<-" + routes[curr]
		}
		r += "<-" + routes[curr]
		log.Debug("full route for %v is %v\n", k, r)
		log.Debug("single hop route for %v is %v\n", k, prev)
		n.routes[k] = prev
	}
}

func (n *Node) dropRoutes() {
	log.Debugln("dropping known routes")
	n.routes = make(map[string]string)
}
