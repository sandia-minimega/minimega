// The minitunnel package implements bidirectional TCP tunnels over any
// io.ReadWriter. It does this in a way similar to the ssh -L command. Tunnel
// construction requires a source port, destination host, and destination port.
//
// minitunnel supports multiple connections and multiple tunnels over a single
// transport.
package minitunnel
