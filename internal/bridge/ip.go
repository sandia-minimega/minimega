// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

// tapFlags are the TUNSETIFF flags that `ip tuntap add mode tap` uses.
const tapFlags = unix.IFF_TAP | unix.IFF_NO_PI | unix.IFF_TUN_EXCL

// reapGroup is the link group that minimega reserves to delete taps in one batch.
const reapGroup = 0x6d6d7470

// upInterface activates an interface through netlink. promisc controls
// whether the interface is brought up in promiscuous mode or not.
func upInterface(name string, promisc bool) error {
	log.Info("up interface: %v", name)

	link, err := netlink.LinkByName(name)
	if err == nil {
		err = netlink.LinkSetUp(link)
	}
	if err == nil && promisc {
		err = netlink.SetPromiscOn(link)
	}
	if err != nil {
		return fmt.Errorf("up interface failed: %v: %w", name, err)
	}

	return nil
}

// downInterface deactivates an interface through netlink.
func downInterface(name string) error {
	log.Info("down interface: %v", name)

	link, err := netlink.LinkByName(name)
	if err == nil {
		err = netlink.LinkSetDown(link)
	}
	if err != nil {
		return fmt.Errorf("down interface failed: %v: %w", name, err)
	}

	return nil
}

// createTap creates a persistent tuntap of the specified name with the same
// flags as `ip tuntap add mode tap`.
func createTap(name string) error {
	log.Info("creating tuntap: %v", name)

	err := addTuntap(name)
	if errors.Is(err, unix.EBUSY) {
		return errAlreadyExists
	} else if err != nil {
		return fmt.Errorf("create tap failed: %v: %w", name, err)
	}

	return nil
}

// addTuntap does the ioctls of iproute2 on /dev/net/tun. It does not use
// netlink.Tuntap because that type always sets an owner and a group.
func addTuntap(name string) error {
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return err
	}
	ifr.SetUint16(tapFlags)

	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		return err
	}

	// without TUNSETPERSIST, the kernel deletes the tap when fd closes
	return unix.IoctlSetInt(fd, unix.TUNSETPERSIST, 1)
}

// createVeth creates a veth of the specified name using the `ip` command.
func createVeth(tap, name, netnsname string) error {
	log.Debug("creating veth: %v on %v in netns %v", name, tap, netnsname)

	args := []string{
		"ip",
		"link",
		"add",
		tap,
		"type",
		"veth",
		"peer",
		name,
		"netns",
		netnsname,
	}

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("create veth failed: %v: %v", err, out)
	}

	return nil
}

// setMAC sets the MAC address for a container interface using the `ip` command.
func setMAC(netnsname, iface, mac string) error {
	log.Debug("setting MAC: %v %v %v", netnsname, iface, mac)

	args := []string{
		"ip",
		"netns",
		"exec",
		netnsname,
		"ip",
		"link",
		"set",
		"dev",
		iface,
		"address",
		mac,
	}

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("set MAC failed: %v: %v", err, out)
	}

	return nil
}

// DestroyTap destroys an `unmanaged` tap through netlink. This can be used
// when cleaning up from a crash or when a tap is not connected to a bridges.
// See `Bridge.DestroyTap` for managed taps.
func DestroyTap(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return destroyTap(name)
}

// destroyTap destroys a tuntap device.
func destroyTap(name string) error {
	log.Info("destroying tuntap: %v", name)

	link, err := netlink.LinkByName(name)
	if err == nil {
		err = netlink.LinkDel(link)
	}
	if err != nil {
		return fmt.Errorf("destroy tap failed: %v: %w", name, err)
	}

	return nil
}

// destroyTaps destroys the tuntap devices with one RTM_DELLINK message for
// reapGroup. If links that are not in names are in reapGroup, it destroys
// each device with its own message. It returns the names of the devices that
// no longer exist, which includes devices that were already gone.
func destroyTaps(names []string) ([]string, error) {
	var (
		gone    []string
		grouped []netlink.Link
		want    = map[string]bool{}
		errs    []error
	)

	for _, name := range names {
		log.Info("destroying tuntap: %v", name)

		link, err := netlink.LinkByName(name)
		if err == nil {
			err = netlink.LinkSetGroup(link, reapGroup)
		}

		switch {
		case isNoDevice(err):
			gone = append(gone, name)
		case err != nil:
			errs = append(errs, fmt.Errorf("destroy tap failed: %v: %w", name, err))
		default:
			grouped = append(grouped, link)
			want[name] = true
		}
	}

	if len(grouped) == 0 {
		return gone, errors.Join(errs...)
	}

	err := checkReapGroup(want)
	if err == nil {
		log.Info("destroying %v tuntaps in link group %v", len(grouped), reapGroup)
		err = delLinkGroup(reapGroup)
	}

	if err == nil || errors.Is(err, unix.ENODEV) {
		for _, link := range grouped {
			gone = append(gone, link.Attrs().Name)
		}

		return gone, errors.Join(errs...)
	}

	log.Warn("cannot destroy tuntaps in link group %v, destroying one at a time: %v", reapGroup, err)

	for _, link := range grouped {
		name := link.Attrs().Name

		if err := netlink.LinkDel(link); err != nil && !isNoDevice(err) {
			errs = append(errs, fmt.Errorf("destroy tap failed: %v: %w", name, err))
			continue
		}

		gone = append(gone, name)
	}

	return gone, errors.Join(errs...)
}

// checkReapGroup returns an error if reapGroup contains a link that is not in
// want or if it cannot list the links.
func checkReapGroup(want map[string]bool) error {
	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("list links: %w", err)
	}

	var others []string
	for _, link := range links {
		attrs := link.Attrs()
		if attrs.Group == reapGroup && !want[attrs.Name] {
			others = append(others, attrs.Name)
		}
	}

	if len(others) > 0 {
		return fmt.Errorf("link group %v also contains %v", reapGroup, others)
	}

	return nil
}

// delLinkGroup deletes all links in group with one RTM_DELLINK message. The
// kernel removes them together with unregister_netdevice_many.
func delLinkGroup(group uint32) error {
	req := nl.NewNetlinkRequest(unix.RTM_DELLINK, unix.NLM_F_ACK)

	// interface index 0 tells the kernel to select the links by IFLA_GROUP
	req.AddData(nl.NewIfInfomsg(unix.AF_UNSPEC))
	req.AddData(nl.NewRtAttr(unix.IFLA_GROUP, nl.Uint32Attr(group)))

	_, err := req.Execute(unix.NETLINK_ROUTE, 0)
	return err
}

// isNoDevice returns true when err reports that a link does not exist.
func isNoDevice(err error) bool {
	var notFound netlink.LinkNotFoundError

	return errors.Is(err, unix.ENODEV) || errors.As(err, &notFound)
}
