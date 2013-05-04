package meshage

import (
	log "minilog"
)

// generateEffectiveNetwork returns the subset of the current known topology that contains
// pairs of known connections. That is, if node A says it's connected to node B
// but node B does not say it's connected to node A, then that connection will
// not be listed in the effective mesh list.
// generateEffectiveNetwork expects to be called with meshLock obtained.
func (n *Node) generateEffectiveNetwork() {
	log.Debugln("generateEffectiveNetwork")

	for {
		emesh := make(mesh)
		for k, v := range n.network {
		effectiveNetworkLoop:
			for _, i := range v {
				for _, j := range emesh[i] {
					if j == k {
						continue effectiveNetworkLoop
					}
				}
				for _, j := range n.network[i] {
					if j == k {
						log.Debug("found pair %v <-> %v", k, i)
						emesh[k] = append(emesh[k], i)
						emesh[i] = append(emesh[i], k)
						break
					}
				}
			}
		}
		n.effectiveNetwork = emesh
		n.routes = make(map[string]string)

		stable := true
		for h, _ := range n.effectiveNetwork {
			if _, ok := n.routes[h]; !ok {
				n.updateRoute(h)
				if _, ok := n.routes[h]; !ok {
					delete(n.network, h)
					stable = false
				}
			}
		}
		if stable {
			break
		}
	}

	log.Debug("new effectiveNetwork: %v", n.effectiveNetwork)
}

// find and record the next hop route for c.
// Additionally, all hops along this route are also the shortest path, so record those as well to
// save on effort.
func (n *Node) updateRoute(c string) {
	//n.meshLock.Lock()
	//defer n.meshLock.Unlock()

	if len(n.effectiveNetwork) == 0 {
		return
	}

	log.Debug("updating route for %v", c)

	routes := make(map[string]string) // a key node has a value of the previous hop, the key exists if it's been visited
	routes[n.name] = n.name

	q := make(chan string, len(n.effectiveNetwork))
	q <- n.name

	for len(q) != 0 {
		v := <-q

		log.Debug("visiting %v", v)

		for _, a := range n.effectiveNetwork[v] {
			if _, ok := routes[a]; !ok {
				q <- a
				log.Debug("previous hop for %v is %v", a, v)
				routes[a] = v
			}
		}

		if v == c {
			break
		}
	}

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
		log.Debug("full route for %v is %v", k, r)
		n.routes[k] = prev
	}
}
