package netcfg

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

var (
	mmdsIP = net.IPv4(169, 254, 169, 254)
	// Temporary link-local address assigned to eth0 so the kernel has a
	// source IP for TCP connections to the MMDS endpoint. Without an
	// address on the interface, connect() hangs because ARP requests are
	// sent with sender 0.0.0.0 and Firecracker's MMDS stack does not
	// respond to ARP probes.
	mmdsTempAddr = &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   net.IPv4(169, 254, 169, 1),
			Mask: net.CIDRMask(16, 32),
		},
	}
)

// SetupMMDS brings up eth0 and prepares it for MMDS access.
//
// The Firecracker docs say the guest only needs:
//
//	ip route add 169.254.169.254 dev eth0
//
// However, that assumes eth0 already has an IP (e.g. from DHCP). When
// booting from initramfs the interface is unconfigured, so we assign a
// temporary link-local address first. This gives the kernel a valid
// source for the TCP handshake with Firecracker's MMDS mini-stack.
// The address is removed by CleanupMMDS after the fetch completes.
func SetupMMDS() error {
	link, err := netlink.LinkByName(defaultInterface)
	if err != nil {
		return fmt.Errorf("mmds: find %s: %w", defaultInterface, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("mmds: link up %s: %w", defaultInterface, err)
	}

	// Assign a temporary link-local address so connect() has a source IP.
	if err := netlink.AddrAdd(link, mmdsTempAddr); err != nil {
		return fmt.Errorf("mmds: add temp addr: %w", err)
	}

	// /32 host route to MMDS endpoint — matches Firecracker docs:
	// "ip route add 169.254.169.254 dev eth0"
	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: mmdsIP, Mask: net.CIDRMask(32, 32)},
		Scope:     netlink.SCOPE_LINK,
	}
	if err := netlink.RouteAdd(route); err != nil {
		// Roll back the temporary address since CleanupMMDS won't be
		// called when SetupMMDS returns an error.
		_ = netlink.AddrDel(link, mmdsTempAddr)
		return fmt.Errorf("mmds: add route: %w", err)
	}
	return nil
}

// CleanupMMDS removes the temporary link-local address and /32 host route
// added by SetupMMDS. Called after MMDS fetch completes (success or failure)
// so the real network configuration can take over cleanly.
func CleanupMMDS() {
	link, err := netlink.LinkByName(defaultInterface)
	if err != nil {
		return
	}
	_ = netlink.RouteDel(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: mmdsIP, Mask: net.CIDRMask(32, 32)},
	})
	_ = netlink.AddrDel(link, mmdsTempAddr)
}
