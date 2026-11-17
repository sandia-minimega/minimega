// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package bridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// Kernel uapi values that golang.org/x/sys/unix does not define.
const (
	vethInfoPeer = 1 // VETH_INFO_PEER
	iflaTunOwner = 1 // IFLA_TUN_OWNER

	tcaMatchallAct = 2 // TCA_MATCHALL_ACT
	tcaActKind     = 1 // TCA_ACT_KIND
	tcaActOptions  = 2 // TCA_ACT_OPTIONS
	tcaMirredParms = 2 // TCA_MIRRED_PARMS
	tcaEgressRedir = 1 // TCA_EGRESS_REDIR
	tcActStolen    = 4 // TC_ACT_STOLEN

	tcHIngress      = 0xfffffff1 // TC_H_INGRESS
	tcIngressHandle = 0xffff0000 // ffff:
	tcFilterPrio    = 1

	sizeofTcMsg    = 20
	sizeofTcMirred = 28
)

var ne = binary.NativeEndian

// nlMsg builds one rtnetlink request.
type nlMsg struct {
	b     []byte
	nests []int
}

func newNLMsg(typ, flags uint16, hdr []byte) *nlMsg {
	m := &nlMsg{b: make([]byte, unix.SizeofNlMsghdr, 256)}
	ne.PutUint16(m.b[4:], typ)
	ne.PutUint16(m.b[6:], flags|unix.NLM_F_REQUEST)
	m.raw(hdr)
	return m
}

func (m *nlMsg) flags() uint16 {
	return ne.Uint16(m.b[6:])
}

func (m *nlMsg) raw(b []byte) {
	m.b = append(m.b, b...)
	for len(m.b)%4 != 0 {
		m.b = append(m.b, 0)
	}
}

func (m *nlMsg) attr(typ uint16, data []byte) {
	m.b = ne.AppendUint16(m.b, uint16(unix.SizeofRtAttr+len(data)))
	m.b = ne.AppendUint16(m.b, typ)
	m.raw(data)
}

func (m *nlMsg) str(typ uint16, s string) {
	m.attr(typ, append([]byte(s), 0))
}

func (m *nlMsg) u32(typ uint16, v uint32) {
	m.attr(typ, ne.AppendUint32(nil, v))
}

// begin starts a nested attribute that end closes.
func (m *nlMsg) begin(typ uint16) {
	m.nests = append(m.nests, len(m.b))
	m.b = ne.AppendUint16(m.b, 0)
	m.b = ne.AppendUint16(m.b, typ)
}

func (m *nlMsg) end() {
	off := m.nests[len(m.nests)-1]
	m.nests = m.nests[:len(m.nests)-1]
	ne.PutUint16(m.b[off:], uint16(len(m.b)-off))
}

func (m *nlMsg) bytes(seq uint32) []byte {
	ne.PutUint32(m.b[0:], uint32(len(m.b)))
	ne.PutUint32(m.b[8:], seq)
	return m.b
}

func ifInfoMsg(index int32, flags, change uint32) []byte {
	b := make([]byte, unix.SizeofIfInfomsg)
	ne.PutUint32(b[4:], uint32(index))
	ne.PutUint32(b[8:], flags)
	ne.PutUint32(b[12:], change)
	return b
}

func tcMsg(ifindex int32, handle, parent, info uint32) []byte {
	b := make([]byte, sizeofTcMsg)
	ne.PutUint32(b[4:], uint32(ifindex))
	ne.PutUint32(b[8:], handle)
	ne.PutUint32(b[12:], parent)
	ne.PutUint32(b[16:], info)
	return b
}

func htons(v uint16) uint16 {
	return ne.Uint16(binary.BigEndian.AppendUint16(nil, v))
}

// tcMirred encodes struct tc_mirred for an egress redirect to ifindex.
func tcMirred(ifindex int32) []byte {
	b := make([]byte, sizeofTcMirred)
	ne.PutUint32(b[8:], tcActStolen)
	ne.PutUint32(b[20:], tcaEgressRedir)
	ne.PutUint32(b[24:], uint32(ifindex))
	return b
}

func vethAddMsg(name, peer string, peerNetnsFD int) *nlMsg {
	m := newNLMsg(unix.RTM_NEWLINK, unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL, ifInfoMsg(0, 0, 0))
	m.str(unix.IFLA_IFNAME, name)
	m.begin(unix.IFLA_LINKINFO)
	m.str(unix.IFLA_INFO_KIND, "veth")
	m.begin(unix.IFLA_INFO_DATA)
	m.begin(vethInfoPeer)
	m.raw(ifInfoMsg(0, 0, 0))
	m.str(unix.IFLA_IFNAME, peer)
	m.u32(unix.IFLA_NET_NS_FD, uint32(peerNetnsFD))
	m.end()
	m.end()
	m.end()
	return m
}

// linkSetMsg sets the MTU of a link when mtu > 0, and sets it up when up is
// true.
func linkSetMsg(name string, mtu int, up bool) *nlMsg {
	var flags uint32
	if up {
		flags = unix.IFF_UP
	}
	m := newNLMsg(unix.RTM_NEWLINK, unix.NLM_F_ACK, ifInfoMsg(0, flags, flags))
	m.str(unix.IFLA_IFNAME, name)
	if mtu > 0 {
		m.u32(unix.IFLA_MTU, uint32(mtu))
	}
	return m
}

func linkGetMsg(name string) *nlMsg {
	m := newNLMsg(unix.RTM_GETLINK, unix.NLM_F_ACK, ifInfoMsg(0, 0, 0))
	m.str(unix.IFLA_IFNAME, name)
	return m
}

func linkDelMsg(name string) *nlMsg {
	m := newNLMsg(unix.RTM_DELLINK, unix.NLM_F_ACK, ifInfoMsg(0, 0, 0))
	m.str(unix.IFLA_IFNAME, name)
	return m
}

func qdiscAddIngressMsg(ifindex int32) *nlMsg {
	m := newNLMsg(unix.RTM_NEWQDISC, unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL, tcMsg(ifindex, tcIngressHandle, tcHIngress, 0))
	m.str(unix.TCA_KIND, "ingress")
	return m
}

// filterAddRedirectMsg adds a matchall filter on the ingress qdisc of ifindex
// with the action mirred egress redirect to target.
func filterAddRedirectMsg(ifindex, target int32) *nlMsg {
	info := uint32(tcFilterPrio)<<16 | uint32(htons(unix.ETH_P_ALL))
	m := newNLMsg(unix.RTM_NEWTFILTER, unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL, tcMsg(ifindex, 0, tcIngressHandle, info))
	m.str(unix.TCA_KIND, "matchall")
	m.begin(unix.TCA_OPTIONS)
	m.begin(tcaMatchallAct)
	m.begin(1)
	m.str(tcaActKind, "mirred")
	m.begin(tcaActOptions)
	m.attr(tcaMirredParms, tcMirred(target))
	m.end()
	m.end()
	m.end()
	m.end()
	return m
}

// nlAttr is one netlink attribute.
type nlAttr struct {
	typ  uint16
	data []byte
}

// nlParseAttrs decodes the attributes in b. The types exclude the nested and
// byte order flags.
func nlParseAttrs(b []byte) ([]nlAttr, error) {
	var res []nlAttr
	for len(b) >= unix.SizeofRtAttr {
		l := int(ne.Uint16(b))
		if l < unix.SizeofRtAttr || l > len(b) {
			return nil, errors.New("bad netlink attribute")
		}
		res = append(res, nlAttr{typ: ne.Uint16(b[2:]) & 0x3fff, data: b[unix.SizeofRtAttr:l]})
		b = b[min(nlAlign(l), len(b)):]
	}
	return res, nil
}

func nlFind(attrs []nlAttr, typ uint16) []byte {
	for _, a := range attrs {
		if a.typ == typ {
			return a.data
		}
	}
	return nil
}

func nlAlign(n int) int {
	return (n + 3) &^ 3
}

func nlString(b []byte) string {
	return strings.TrimRight(string(b), "\x00")
}

// nlConn is a NETLINK_ROUTE socket in the network namespace of the thread
// that opened it.
type nlConn struct {
	fd  int
	seq uint32
	buf []byte
}

func nlDial() (*nlConn, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bind netlink socket: %w", err)
	}

	// Error messages and short acks are optional.
	unix.SetsockoptInt(fd, unix.SOL_NETLINK, unix.NETLINK_EXT_ACK, 1)
	unix.SetsockoptInt(fd, unix.SOL_NETLINK, unix.NETLINK_CAP_ACK, 1)

	return &nlConn{fd: fd, buf: make([]byte, 1<<16)}, nil
}

func (c *nlConn) Close() error {
	return unix.Close(c.fd)
}

// do sends m and reads until the ack or the end of a dump. It calls fn for
// each other reply.
func (c *nlConn) do(op string, m *nlMsg, fn func(typ uint16, data []byte) error) error {
	c.seq++
	seq := c.seq

	if err := unix.Sendto(c.fd, m.bytes(seq), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return fmt.Errorf("%v: %w", op, err)
	}

	for {
		n, _, err := unix.Recvfrom(c.fd, c.buf, 0)
		if err == unix.EINTR {
			continue
		} else if err != nil {
			return fmt.Errorf("%v: %w", op, err)
		}

		msgs := c.buf[:n]
		for len(msgs) >= unix.SizeofNlMsghdr {
			l := int(ne.Uint32(msgs))
			typ, flags, mseq := ne.Uint16(msgs[4:]), ne.Uint16(msgs[6:]), ne.Uint32(msgs[8:])
			if l < unix.SizeofNlMsghdr || l > len(msgs) {
				return fmt.Errorf("%v: bad netlink message", op)
			}
			data := msgs[unix.SizeofNlMsghdr:l]
			msgs = msgs[min(nlAlign(l), len(msgs)):]

			if mseq != seq {
				continue
			}

			switch typ {
			case unix.NLMSG_ERROR, unix.NLMSG_DONE:
				if len(data) < 4 {
					if typ == unix.NLMSG_DONE {
						return nil
					}
					return fmt.Errorf("%v: short netlink error", op)
				}
				if errno := int32(ne.Uint32(data)); errno != 0 {
					return nlError(op, unix.Errno(-errno), typ, flags, data)
				}
				return nil
			default:
				if fn != nil {
					if err := fn(typ, data); err != nil {
						return fmt.Errorf("%v: %w", op, err)
					}
				}
			}
		}
	}
}

// nlError returns errno with the extended ack message, if any.
func nlError(op string, errno unix.Errno, typ, flags uint16, data []byte) error {
	off := 4
	if typ == unix.NLMSG_ERROR {
		off += unix.SizeofNlMsghdr
		if flags&unix.NLM_F_CAPPED == 0 && len(data) >= off {
			off = 4 + int(ne.Uint32(data[4:]))
		}
	}

	if flags&unix.NLM_F_ACK_TLVS != 0 && off <= len(data) {
		if attrs, err := nlParseAttrs(data[off:]); err == nil {
			if msg := nlString(nlFind(attrs, unix.NLMSGERR_ATTR_MSG)); msg != "" {
				return fmt.Errorf("%v: %w: %v", op, errno, msg)
			}
		}
	}

	return fmt.Errorf("%v: %w", op, errno)
}

// nlLink is the part of a link that minimega reads.
type nlLink struct {
	Index    int32
	Up       bool
	MTU      int
	Kind     string
	TunOwner int // -1 when not a tun device with an owner
}

func parseLink(data []byte) (nlLink, error) {
	l := nlLink{TunOwner: -1}
	if len(data) < unix.SizeofIfInfomsg {
		return l, errors.New("short link message")
	}
	l.Index = int32(ne.Uint32(data[4:]))
	l.Up = ne.Uint32(data[8:])&unix.IFF_UP != 0

	attrs, err := nlParseAttrs(data[unix.SizeofIfInfomsg:])
	if err != nil {
		return l, err
	}
	if b := nlFind(attrs, unix.IFLA_MTU); len(b) == 4 {
		l.MTU = int(ne.Uint32(b))
	}
	if info, err := nlParseAttrs(nlFind(attrs, unix.IFLA_LINKINFO)); err == nil {
		l.Kind = nlString(nlFind(info, unix.IFLA_INFO_KIND))
		if d, err := nlParseAttrs(nlFind(info, unix.IFLA_INFO_DATA)); err == nil && l.Kind == "tun" {
			if b := nlFind(d, iflaTunOwner); len(b) == 4 {
				l.TunOwner = int(ne.Uint32(b))
			}
		}
	}

	return l, nil
}

func (c *nlConn) linkAddVethPeerNetns(name, peer string, ns *Netns) error {
	return c.do("create veth "+name, vethAddMsg(name, peer, ns.fd), nil)
}

func (c *nlConn) linkSet(name string, mtu int, up bool) error {
	return c.do("set link "+name, linkSetMsg(name, mtu, up), nil)
}

func (c *nlConn) linkGet(name string) (nlLink, error) {
	var res nlLink
	var found bool
	err := c.do("get link "+name, linkGetMsg(name), func(typ uint16, data []byte) error {
		if typ != unix.RTM_NEWLINK {
			return nil
		}
		l, err := parseLink(data)
		if err != nil {
			return err
		}
		res, found = l, true
		return nil
	})
	if err == nil && !found {
		err = fmt.Errorf("get link %v: no reply", name)
	}
	return res, err
}

func (c *nlConn) linkDel(name string) error {
	return c.do("delete link "+name, linkDelMsg(name), nil)
}

func (c *nlConn) qdiscAddIngress(ifindex int32) error {
	return c.do(fmt.Sprintf("add ingress qdisc to link %d", ifindex), qdiscAddIngressMsg(ifindex), nil)
}

func (c *nlConn) filterAddRedirect(ifindex, target int32) error {
	return c.do(fmt.Sprintf("add redirect filter from link %d to %d", ifindex, target), filterAddRedirectMsg(ifindex, target), nil)
}

// createTunTap creates the persistent tap name, owned by uid, in the network
// namespace of the calling thread. It sets no group, so a VMM that runs as uid
// can open the tap with any gid.
func createTunTap(name string, uid int) error {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("create tap %v: %w", name, err)
	}
	defer unix.Close(fd)

	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return fmt.Errorf("create tap %v: %w", name, err)
	}
	ifr.SetUint16(unix.IFF_TAP | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		return fmt.Errorf("create tap %v: %w", name, err)
	}

	for _, req := range []struct {
		op uint
		v  int
	}{
		{unix.TUNSETOWNER, uid},
		{unix.TUNSETPERSIST, 1},
	} {
		if err := unix.IoctlSetInt(fd, req.op, req.v); err != nil {
			return fmt.Errorf("set up tap %v: %w", name, err)
		}
	}

	return nil
}
