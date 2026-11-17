// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package bridge

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// nlTestMsg splits an encoded request into its header fields, fixed header,
// and attributes.
type nlTestMsg struct {
	typ, flags uint16
	seq        uint32
	hdr        []byte
	attrs      []nlAttr
}

func decodeNLTest(t *testing.T, b []byte, hdrLen int) nlTestMsg {
	t.Helper()

	if len(b)%4 != 0 {
		t.Fatalf("message length %d is not aligned", len(b))
	}
	if l := int(ne.Uint32(b)); l != len(b) {
		t.Fatalf("header length %d, want %d", l, len(b))
	}
	m := nlTestMsg{
		typ:   ne.Uint16(b[4:]),
		flags: ne.Uint16(b[6:]),
		seq:   ne.Uint32(b[8:]),
		hdr:   b[16 : 16+hdrLen],
	}
	attrs, err := nlParseAttrs(b[16+hdrLen:])
	if err != nil {
		t.Fatal(err)
	}
	m.attrs = attrs
	return m
}

func mustAttrs(t *testing.T, b []byte) []nlAttr {
	t.Helper()

	attrs, err := nlParseAttrs(b)
	if err != nil {
		t.Fatal(err)
	}
	return attrs
}

func attrTypes(attrs []nlAttr) []uint16 {
	var res []uint16
	for _, a := range attrs {
		res = append(res, a.typ)
	}
	return res
}

func TestNetlinkMsgLinkSet(t *testing.T) {
	var want []byte
	want = ne.AppendUint32(want, 16+16+8+8)
	want = ne.AppendUint16(want, unix.RTM_NEWLINK)
	want = ne.AppendUint16(want, unix.NLM_F_REQUEST|unix.NLM_F_ACK)
	want = ne.AppendUint32(want, 9)
	want = ne.AppendUint32(want, 0)
	want = append(want, 0, 0, 0, 0)
	want = ne.AppendUint32(want, 0)
	want = ne.AppendUint32(want, unix.IFF_UP)
	want = ne.AppendUint32(want, unix.IFF_UP)
	want = ne.AppendUint16(want, 8)
	want = ne.AppendUint16(want, unix.IFLA_IFNAME)
	want = append(want, 'f', 'c', '0', 0)
	want = ne.AppendUint16(want, 8)
	want = ne.AppendUint16(want, unix.IFLA_MTU)
	want = ne.AppendUint32(want, 1450)

	if got := linkSetMsg("fc0", 1450, true).bytes(9); !bytes.Equal(got, want) {
		t.Errorf("linkSetMsg() =\n% x\nwant\n% x", got, want)
	}

	m := decodeNLTest(t, linkSetMsg("mega_tap7", 0, true).bytes(1), unix.SizeofIfInfomsg)
	if !reflect.DeepEqual(attrTypes(m.attrs), []uint16{unix.IFLA_IFNAME}) {
		t.Errorf("linkSetMsg without MTU attributes = %v", attrTypes(m.attrs))
	}
	if string(m.attrs[0].data) != "mega_tap7\x00" {
		t.Errorf("name = %q", m.attrs[0].data)
	}
}

func TestNetlinkMsgVeth(t *testing.T) {
	m := decodeNLTest(t, vethAddMsg("mega_tap7", "vm0", 42).bytes(3), unix.SizeofIfInfomsg)

	if m.typ != unix.RTM_NEWLINK || m.seq != 3 {
		t.Errorf("type %d seq %d", m.typ, m.seq)
	}
	if want := uint16(unix.NLM_F_REQUEST | unix.NLM_F_ACK | unix.NLM_F_CREATE | unix.NLM_F_EXCL); m.flags != want {
		t.Errorf("flags %#x, want %#x", m.flags, want)
	}
	if !bytes.Equal(m.hdr, make([]byte, 16)) {
		t.Errorf("ifinfomsg = % x", m.hdr)
	}
	if !reflect.DeepEqual(attrTypes(m.attrs), []uint16{unix.IFLA_IFNAME, unix.IFLA_LINKINFO}) {
		t.Fatalf("attributes = %v", attrTypes(m.attrs))
	}
	if string(m.attrs[0].data) != "mega_tap7\x00" {
		t.Errorf("name = %q", m.attrs[0].data)
	}

	info := mustAttrs(t, m.attrs[1].data)
	if !reflect.DeepEqual(attrTypes(info), []uint16{unix.IFLA_INFO_KIND, unix.IFLA_INFO_DATA}) {
		t.Fatalf("link info = %v", attrTypes(info))
	}
	if string(info[0].data) != "veth\x00" {
		t.Errorf("kind = %q", info[0].data)
	}

	data := mustAttrs(t, info[1].data)
	if len(data) != 1 || data[0].typ != vethInfoPeer {
		t.Fatalf("veth data = %v", attrTypes(data))
	}
	peer := data[0].data
	if len(peer) < 16 || !bytes.Equal(peer[:16], make([]byte, 16)) {
		t.Fatalf("peer ifinfomsg = % x", peer)
	}
	pa := mustAttrs(t, peer[16:])
	if !reflect.DeepEqual(attrTypes(pa), []uint16{unix.IFLA_IFNAME, unix.IFLA_NET_NS_FD}) {
		t.Fatalf("peer attributes = %v", attrTypes(pa))
	}
	if string(pa[0].data) != "vm0\x00" || ne.Uint32(pa[1].data) != 42 {
		t.Errorf("peer = %q, netns fd %d", pa[0].data, ne.Uint32(pa[1].data))
	}
}

func TestNetlinkMsgLinkGetDel(t *testing.T) {
	for typ, b := range map[uint16][]byte{
		unix.RTM_GETLINK: linkGetMsg("vm1").bytes(1),
		unix.RTM_DELLINK: linkDelMsg("vm1").bytes(1),
	} {
		m := decodeNLTest(t, b, unix.SizeofIfInfomsg)
		if m.typ != typ || m.flags != unix.NLM_F_REQUEST|unix.NLM_F_ACK {
			t.Errorf("type %d flags %#x", m.typ, m.flags)
		}
		if len(m.attrs) != 1 || m.attrs[0].typ != unix.IFLA_IFNAME || string(m.attrs[0].data) != "vm1\x00" {
			t.Errorf("type %d attributes = %v", typ, m.attrs)
		}
	}
}

func TestNetlinkMsgQdisc(t *testing.T) {
	var want []byte
	want = ne.AppendUint32(want, 16+20+12)
	want = ne.AppendUint16(want, unix.RTM_NEWQDISC)
	want = ne.AppendUint16(want, unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL)
	want = ne.AppendUint32(want, 2)
	want = ne.AppendUint32(want, 0)
	want = append(want, 0, 0, 0, 0)
	want = ne.AppendUint32(want, 5)
	want = ne.AppendUint32(want, 0xffff0000)
	want = ne.AppendUint32(want, 0xfffffff1)
	want = ne.AppendUint32(want, 0)
	want = ne.AppendUint16(want, 12)
	want = ne.AppendUint16(want, unix.TCA_KIND)
	want = append(want, "ingress\x00"...)

	if got := qdiscAddIngressMsg(5).bytes(2); !bytes.Equal(got, want) {
		t.Errorf("qdiscAddIngressMsg() =\n% x\nwant\n% x", got, want)
	}
}

// mirredTarget returns the redirect target in the TCA_OPTIONS of a matchall
// filter, or -1.
func mirredTarget(t *testing.T, opts []byte) (int32, uint32) {
	t.Helper()

	acts := mustAttrs(t, nlFind(mustAttrs(t, opts), tcaMatchallAct))
	act := mustAttrs(t, nlFind(acts, 1))
	if nlString(nlFind(act, tcaActKind)) != "mirred" {
		return -1, 0
	}
	p := nlFind(mustAttrs(t, nlFind(act, tcaActOptions)), tcaMirredParms)
	if len(p) < sizeofTcMirred {
		return -1, 0
	}
	if ne.Uint32(p[20:]) != tcaEgressRedir {
		return -1, 0
	}
	return int32(ne.Uint32(p[24:])), ne.Uint32(p[8:])
}

func TestNetlinkMsgFilter(t *testing.T) {
	m := decodeNLTest(t, filterAddRedirectMsg(3, 5).bytes(4), sizeofTcMsg)

	if m.typ != unix.RTM_NEWTFILTER || m.flags != unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL {
		t.Errorf("type %d flags %#x", m.typ, m.flags)
	}

	ifindex, handle, parent, info := ne.Uint32(m.hdr[4:]), ne.Uint32(m.hdr[8:]), ne.Uint32(m.hdr[12:]), ne.Uint32(m.hdr[16:])
	if ifindex != 3 || handle != 0 || parent != 0xffff0000 {
		t.Errorf("tcmsg ifindex %d handle %#x parent %#x", ifindex, handle, parent)
	}
	if prio, proto := info>>16, binary.BigEndian.Uint16(ne.AppendUint16(nil, uint16(info))); prio != 1 || proto != unix.ETH_P_ALL {
		t.Errorf("tcm_info %#x: prio %d protocol %#x", info, prio, proto)
	}

	if !reflect.DeepEqual(attrTypes(m.attrs), []uint16{unix.TCA_KIND, unix.TCA_OPTIONS}) {
		t.Fatalf("attributes = %v", attrTypes(m.attrs))
	}
	if string(m.attrs[0].data) != "matchall\x00" {
		t.Errorf("kind = %q", m.attrs[0].data)
	}

	p := nlFind(mustAttrs(t, nlFind(mustAttrs(t, nlFind(mustAttrs(t, nlFind(mustAttrs(t, m.attrs[1].data), tcaMatchallAct)), 1)), tcaActOptions)), tcaMirredParms)
	var want []byte
	for _, v := range []uint32{0, 0, tcActStolen, 0, 0, tcaEgressRedir, 5} {
		want = ne.AppendUint32(want, v)
	}
	if !bytes.Equal(p, want) {
		t.Errorf("tc_mirred = % x, want % x", p, want)
	}
	if target, action := mirredTarget(t, m.attrs[1].data); target != 5 || action != tcActStolen {
		t.Errorf("redirect target %d action %d", target, action)
	}
}

func TestNetlinkParseLink(t *testing.T) {
	m := newNLMsg(unix.RTM_NEWLINK, 0, ifInfoMsg(7, unix.IFF_UP, 0))
	m.str(unix.IFLA_IFNAME, "fc0")
	m.u32(unix.IFLA_MTU, 9000)
	m.begin(unix.IFLA_LINKINFO)
	m.str(unix.IFLA_INFO_KIND, "tun")
	m.begin(unix.IFLA_INFO_DATA)
	m.u32(iflaTunOwner, 2000000007)
	m.end()
	m.end()

	l, err := parseLink(m.bytes(1)[16:])
	if err != nil {
		t.Fatal(err)
	}
	if want := (nlLink{Index: 7, Up: true, MTU: 9000, Kind: "tun", TunOwner: 2000000007}); l != want {
		t.Errorf("parseLink() = %+v, want %+v", l, want)
	}

	if _, err := parseLink(make([]byte, 8)); err == nil {
		t.Error("short message: no error")
	}
	if _, err := nlParseAttrs([]byte{9, 0, 1, 0}); err == nil {
		t.Error("bad attribute length: no error")
	}
}

func TestNetlinkError(t *testing.T) {
	errno := -int32(unix.EEXIST)
	data := ne.AppendUint32(nil, uint32(errno))
	data = append(data, make([]byte, 16)...)
	tlv := newNLMsg(0, 0, nil)
	tlv.str(unix.NLMSGERR_ATTR_MSG, "Exclusivity flag on, cannot modify")
	data = append(data, tlv.bytes(0)[16:]...)

	err := nlError("add filter", unix.EEXIST, unix.NLMSG_ERROR, unix.NLM_F_CAPPED|unix.NLM_F_ACK_TLVS, data)
	if !errors.Is(err, unix.EEXIST) || !strings.Contains(err.Error(), "add filter") || !strings.Contains(err.Error(), "Exclusivity flag") {
		t.Errorf("nlError() = %v", err)
	}

	if err := nlError("get link", unix.ENODEV, unix.NLMSG_ERROR, 0, data[:20]); !errors.Is(err, unix.ENODEV) || err.Error() != "get link: no such device" {
		t.Errorf("nlError() without TLVs = %v", err)
	}
}

// tcDump returns the attributes of each qdisc or filter on ifindex.
func tcDump(c *nlConn, typ uint16, ifindex int32, parent uint32) ([][]nlAttr, error) {
	var res [][]nlAttr
	err := c.do("dump", newNLMsg(typ, unix.NLM_F_DUMP, tcMsg(ifindex, 0, parent, 0)), func(_ uint16, data []byte) error {
		if len(data) < sizeofTcMsg {
			return errors.New("short tc message")
		}
		if int32(ne.Uint32(data[4:])) != ifindex {
			return nil
		}
		attrs, err := nlParseAttrs(data[sizeofTcMsg:])
		if err != nil {
			return err
		}
		res = append(res, attrs)
		return nil
	})
	return res, err
}

func runNetns(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("/run/netns")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var res []string
	for _, e := range entries {
		res = append(res, e.Name())
	}
	return res
}

func checkNetnsLinks(t *testing.T, ns *Netns, index, uid, mtu int) {
	t.Helper()

	err := ns.Do(func() error {
		c, err := nlDial()
		if err != nil {
			return err
		}
		defer c.Close()

		veth, err := c.linkGet(fmt.Sprintf("vm%d", index))
		if err != nil {
			return err
		}
		tap, err := c.linkGet(fmt.Sprintf("fc%d", index))
		if err != nil {
			return err
		}

		if veth.Kind != "veth" || !veth.Up || veth.MTU != mtu {
			t.Errorf("netlink_integration: vm%d = %+v, want an up veth with MTU %d", index, veth, mtu)
		}
		if tap.Kind != "tun" || !tap.Up || tap.MTU != mtu || tap.TunOwner != uid {
			t.Errorf("netlink_integration: fc%d = %+v, want an up tun with MTU %d and owner %d", index, tap, mtu, uid)
		}

		for _, p := range [][2]nlLink{{veth, tap}, {tap, veth}} {
			qdiscs, err := tcDump(c, unix.RTM_GETQDISC, p[0].Index, 0)
			if err != nil {
				return err
			}
			var ingress bool
			for _, q := range qdiscs {
				ingress = ingress || nlString(nlFind(q, unix.TCA_KIND)) == "ingress"
			}
			if !ingress {
				t.Errorf("netlink_integration: link %d has no ingress qdisc", p[0].Index)
			}

			filters, err := tcDump(c, unix.RTM_GETTFILTER, p[0].Index, tcIngressHandle)
			if err != nil {
				return err
			}
			var redirect bool
			for _, f := range filters {
				if nlString(nlFind(f, unix.TCA_KIND)) != "matchall" {
					continue
				}
				if opts := nlFind(f, unix.TCA_OPTIONS); opts != nil {
					if target, action := mirredTarget(t, opts); target == p[1].Index && action == tcActStolen {
						redirect = true
					}
				}
			}
			if !redirect {
				t.Errorf("netlink_integration: link %d has no matchall redirect to %d: %v", p[0].Index, p[1].Index, filters)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("netlink_integration: check netns links: %v", err)
	}
}

func hostLink(t *testing.T, name string) (nlLink, error) {
	t.Helper()

	c, err := nlDial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	return c.linkGet(name)
}

// TestNetlinkIntegration creates netns taps against the kernel. It needs root
// and MM_NETLINK_TEST=1.
func TestNetlinkIntegration(t *testing.T) {
	if os.Getuid() != 0 || os.Getenv("MM_NETLINK_TEST") != "1" {
		t.Skip("needs root and MM_NETLINK_TEST=1")
	}

	if _, err := os.Stat("/dev/net/tun"); os.IsNotExist(err) {
		if err := os.MkdirAll("/dev/net", 0755); err != nil {
			t.Fatal(err)
		}
		if err := unix.Mknod("/dev/net/tun", unix.S_IFCHR|0666, int(unix.Mkdev(10, 200))); err != nil {
			t.Fatal(err)
		}
	}

	oldAdd, oldBatch := netnsAddTap, OVSBatching
	t.Cleanup(func() { netnsAddTap, OVSBatching = oldAdd, oldBatch })
	OVSBatching = false
	netnsAddTap = func(b *Bridge, tap, mac string, vlan int) error {
		b.taps[tap] = &Tap{Name: tap, Bridge: b.Name, VLAN: vlan, MAC: mac}
		return nil
	}

	before := runNetns(t)

	tapChan := make(chan string, 2)
	tapChan <- "nltest_tap0"
	tapChan <- "nltest_tap1"
	b := &Bridge{Name: "nltest", taps: map[string]*Tap{}, tapChan: tapChan}
	const uid = 2000000007

	ns, err := NewNetns()
	if err != nil {
		t.Fatalf("netlink_integration: NewNetns: %v", err)
	}
	defer ns.Close()

	if p := ns.Path(); !strings.HasPrefix(p, fmt.Sprintf("/proc/%d/fd/", os.Getpid())) {
		t.Errorf("netlink_integration: Path() = %q", p)
	}
	var self, other unix.Stat_t
	if err := unix.Stat(netnsSelf, &self); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat(ns.Path(), &other); err != nil {
		t.Fatal(err)
	}
	if self.Ino == other.Ino {
		t.Fatal("netlink_integration: the new netns is the netns of the test")
	}

	tap, mtu, err := b.CreateNetnsTap(ns, "00:11:22:33:44:55", 101, 0, uid)
	if err != nil {
		t.Fatalf("netlink_integration: CreateNetnsTap: %v", err)
	}
	if tap != "nltest_tap0" || mtu != 1500 {
		t.Errorf("netlink_integration: CreateNetnsTap() = %q, %d", tap, mtu)
	}
	if got := b.taps[tap]; got == nil || got.VLAN != 101 {
		t.Errorf("netlink_integration: tap table entry = %+v", got)
	}

	host, err := hostLink(t, tap)
	if err != nil || host.Kind != "veth" || !host.Up || host.MTU != mtu {
		t.Errorf("netlink_integration: host link = %+v, %v", host, err)
	}
	if _, err := hostLink(t, "vm0"); !errors.Is(err, unix.ENODEV) {
		t.Errorf("netlink_integration: vm0 in the host netns: %v", err)
	}

	checkNetnsLinks(t, ns, 0, uid, mtu)

	// A failed tap leaves no host end.
	if _, _, err := b.CreateNetnsTap(ns, "00:11:22:33:44:56", 101, 0, uid); err == nil {
		t.Error("netlink_integration: a second fc0: no error")
	}
	if _, err := hostLink(t, "nltest_tap1"); !errors.Is(err, unix.ENODEV) {
		t.Errorf("netlink_integration: host end of the failed tap: %v", err)
	}

	tapChan <- "nltest_tap2"
	tap2, _, err := b.CreateNetnsTap(ns, "00:11:22:33:44:57", 102, 1, uid)
	if err != nil {
		t.Fatalf("netlink_integration: CreateNetnsTap 1: %v", err)
	}
	checkNetnsLinks(t, ns, 1, uid, mtu)
	if err := b.DestroyNetnsTap(tap2); err != nil {
		t.Errorf("netlink_integration: DestroyNetnsTap: %v", err)
	}
	if _, err := hostLink(t, tap2); !errors.Is(err, unix.ENODEV) {
		t.Errorf("netlink_integration: host end after DestroyNetnsTap: %v", err)
	}
	if !b.taps[tap2].Defunct {
		t.Error("netlink_integration: tap is not defunct")
	}

	if err := ns.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ns.Do(func() error { return nil }); err == nil {
		t.Error("netlink_integration: Do after Close: no error")
	}

	// The kernel removes the namespace, and the veth pair with it, after the
	// last reference goes.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := hostLink(t, tap)
		if errors.Is(err, unix.ENODEV) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("netlink_integration: host link %v after Close: %v", tap, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if after := runNetns(t); !reflect.DeepEqual(after, before) {
		t.Errorf("netlink_integration: /run/netns = %v, before %v", after, before)
	}
}
