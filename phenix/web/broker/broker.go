package broker

import "encoding/json"

var (
	clients    = make(map[*Client]bool)
	broadcast  = make(chan Publish)
	register   = make(chan *Client)
	unregister = make(chan *Client)
)

func Start() {
	for {
		select {
		case cli := <-register:
			clients[cli] = true
		case cli := <-unregister:
			if _, ok := clients[cli]; ok {
				cli.Stop()
				delete(clients, cli)
			}
		case pub := <-broadcast:
			for cli := range clients {
				var (
					policy = pub.RequestPolicy
					allow  bool
				)

				if policy == nil {
					allow = true
				} else if policy.ResourceName == "" {
					allow = cli.role.Allowed(policy.Resource, policy.Verb)
				} else {
					allow = cli.role.Allowed(policy.Resource, policy.Verb, policy.ResourceName)
				}

				if allow {
					select {
					case cli.publish <- pub:
					default:
						cli.Stop()
						delete(clients, cli)
					}
				}
			}
		}
	}
}

func Broadcast(policy *RequestPolicy, resource *Resource, msg json.RawMessage) {
	broadcast <- Publish{RequestPolicy: policy, Resource: resource, Result: msg}
}
