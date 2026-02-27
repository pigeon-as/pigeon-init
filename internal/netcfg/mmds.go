package netcfg

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var mmdsIP = net.IPv4(169, 254, 169, 254)

var mmdsAddr = &netlink.Addr{
	IPNet: &net.IPNet{
		IP:   net.IPv4(169, 254, 0, 1),
		Mask: net.CIDRMask(16, 32),
	},
	Flags: unix.IFA_F_NODAD,
}

// SetupMMDS brings up eth0 with a link-local address and adds a host
// route to the MMDS endpoint (required by Firecracker).
func SetupMMDS() error {
	link, err := netlink.LinkByName(defaultInterface)
	if err != nil {
		return fmt.Errorf("mmds: find %s: %w", defaultInterface, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("mmds: link up %s: %w", defaultInterface, err)
	}
	if err := netlink.AddrAdd(link, mmdsAddr); err != nil {
		return fmt.Errorf("mmds: add addr: %w", err)
	}
	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: mmdsIP, Mask: net.CIDRMask(32, 32)},
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("mmds: add route: %w", err)
	}
	return nil
}

func CleanupMMDS() {
	link, err := netlink.LinkByName(defaultInterface)
	if err != nil {
		return
	}
	_ = netlink.RouteDel(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: mmdsIP, Mask: net.CIDRMask(32, 32)},
	})
	_ = netlink.AddrDel(link, mmdsAddr)
}
