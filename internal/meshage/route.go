// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package meshage

import (
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
			for _, i := range v { // for each connection i to node k, see if i also reports being connected to k
				for _, j := range emesh[i] { // do we already have this connection noted? if so, move on. This should happen zero or one times for each node.
					if j == k {
						continue effectiveNetworkLoop
					}
				}
				for _, j := range n.network[i] { // go through all of node i's connections looking for k
					if j == k {
						if log.WillLog(log.DEBUG) {
							log.Debug("found pair %v <-> %v", k, i)
						}
						// note the connection in the adjacency list for both i and k
						emesh[k] = append(emesh[k], i)
						emesh[i] = append(emesh[i], k)
						break
					}
				}
			}
		}
		n.effectiveNetwork = emesh

		// now generate routes to each of the nodes from us based on the effectiveNetwork
		n.routes = make(map[string]string)

		// attempt to learn routes to each node from this node,
		// assuming that all nodes in the effective network are
		// routable.  It's possible that the effective network
		// represents partitioned meshes, so if we cannot find a route
		// to a node, remove it from the known network and start this
		// entire process over.
		stable := true
		for h, _ := range n.effectiveNetwork {
			if _, ok := n.routes[h]; !ok {
				n.updateRoute(h)
				if _, ok := n.routes[h]; !ok {
					log.Debug("removing unroutable node %v", h)
					delete(n.network, h)
					stable = false
				}
			}
		}
		if stable {
			break
		}
	}

	if log.WillLog(log.DEBUG) {
		log.Debug("new effectiveNetwork: %v", n.effectiveNetwork)
	}
}

// find and record the next hop route for c.
// Additionally, all hops along this route are also the shortest path, so
// record those as well to save on effort.
func (n *Node) updateRoute(c string) {
	if len(n.effectiveNetwork) == 0 {
		return
	}

	if log.WillLog(log.DEBUG) {
		log.Debug("updating route for %v", c)
	}

	routes := make(map[string]string) // a key node has a value of the previous hop, the key exists if it's been visited
	routes[n.name] = n.name           // the route to ourself is pretty easy to calculate

	// dijkstra's algorithm is well suited in go - we can use a buffered
	// channel of nodes to order our search. We start by putting ourselves
	// in the queue (channel) and follow all nodes connected to it. If we
	// haven't visited that node before, it goes in the queue.
	q := make(chan string, len(n.effectiveNetwork))
	q <- n.name

	for len(q) != 0 {
		v := <-q

		if log.WillLog(log.DEBUG) {
			log.Debug("visiting %v", v)
		}

		for _, a := range n.effectiveNetwork[v] {
			if _, ok := routes[a]; !ok {
				q <- a
				if log.WillLog(log.DEBUG) {
					log.Debug("previous hop for %v is %v", a, v)
				}
				routes[a] = v // this is the route to node a from v
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
		if curr == n.name {
			r += "<-" + routes[curr]
		}
		for curr != n.name {
			prev = curr
			curr = routes[curr]
			r += "<-" + routes[curr]
		}
		if log.WillLog(log.DEBUG) {
			log.Debug("full route for %v is %v", k, r)
		}
		n.routes[k] = prev
	}
}
