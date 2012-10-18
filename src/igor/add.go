package main

import (
)

var cmdAdd = &Command{
	UsageLine: "add -r <reservation name> { -n <integer> | -w <node list> }",
	Short:	"add nodes to specified reservation",
	Long:`
Add adds the specified nodes to the specified reservation.

The reservation name must be specified and must be an existing reservation. Either the -n or -w flag must also be specified.

The -r flag selects the reservation.

The -n flag indicates that the specified number of nodes should be added to the reservation. The first available nodes will be allocated.

The -w flag specifies that the given nodes should be added to the reservation. This will return an error if the nodes are already reserved.
	`,
}
