package broker

import "encoding/json"

type RequestPolicy struct {
	Resource     string
	ResourceName string
	Verb         string
}

func NewRequestPolicy(r, rn, v string) *RequestPolicy {
	return &RequestPolicy{Resource: r, ResourceName: rn, Verb: v}
}

type Resource struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

func NewResource(t, n, a string) *Resource {
	return &Resource{Type: t, Name: n, Action: a}
}

type Publish struct {
	RequestPolicy *RequestPolicy  `json:"-"`
	Resource      *Resource       `json:"resource"`
	Result        json.RawMessage `json:"result"`
}

type Request struct {
	Resource *Resource       `json:"resource"`
	Payload  json.RawMessage `json:"request"`
}

/*
Request:

{
	"resource": {
		"type": "experiment/vms",
		"name": "<exp name>",
		"action": "list"
	},
	"request": {
		"sort_column": "name",
		"sort_asc": true,
		"page_number": 1,
		"page_size": 5
	}
}

Response:

{
	"resource": {
		"type": "experiment/vms",
		"name": "<exp name>",
		"action": "list"
	},
	"result": {
		"vms": [
			...
		]
	}
}

Screenshot Updates:

{
	"resource": {
		"type": "experiment/vm/screenshot",
		"name": "<exp name>/<vm name>",
		"action": "update"
	},
	"result": {
		"screenshot": "data:image/png;base64,..."
	}
}
*/
