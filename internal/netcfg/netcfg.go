package netcfg

import (
	"fmt"
	"net"
	"unsafe"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

const (
	defaultMTU       = 1500
	defaultInterface = "eth0"
)

func Configure(ipConfigs []config.IPConfig, mtu int) error {
	if lo, err := netlink.LinkByName("lo"); err == nil {
		_ = netlink.LinkSetUp(lo)
	}

	if len(ipConfigs) == 0 {
		return nil
	}

	eth0, err := netlink.LinkByName(defaultInterface)
	if err != nil {
		return fmt.Errorf("find %s: %w", defaultInterface, err)
	}

	if mtu <= 0 {
		mtu = defaultMTU
	}
	if err := netlink.LinkSetMTU(eth0, mtu); err != nil {
		return fmt.Errorf("set MTU on %s: %w", defaultInterface, err)
	}
	if err := netlink.LinkSetUp(eth0); err != nil {
		return fmt.Errorf("link up %s: %w", defaultInterface, err)
	}

	// Disable rx/tx checksum offload (Firecracker virtio-net).
	if err := disableChecksums(defaultInterface); err != nil {
		return err
	}

	for _, ipc := range ipConfigs {
		if err := addAddress(eth0, ipc); err != nil {
			return err
		}
		if err := addRoute(eth0, ipc); err != nil {
			return err
		}
	}

	return nil
}

func addAddress(link netlink.Link, ipc config.IPConfig) error {
	ip, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ipc.IP, ipc.Mask))
	if err != nil {
		return fmt.Errorf("parse IP %s/%d: %w", ipc.IP, ipc.Mask, err)
	}
	ipNet.IP = ip

	addr := &netlink.Addr{
		IPNet: ipNet,
		Flags: unix.IFA_F_NODAD,
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("add addr %s: %w", ipNet, err)
	}

	return nil
}

func addRoute(link netlink.Link, ipc config.IPConfig) error {
	gw, _, err := net.ParseCIDR(ipc.Gateway)
	if err != nil {
		gw = net.ParseIP(ipc.Gateway)
		if gw == nil {
			return fmt.Errorf("parse gateway %s: invalid", ipc.Gateway)
		}
	}

	ip := net.ParseIP(ipc.IP)
	if ip == nil {
		return fmt.Errorf("parse IP %s: invalid", ipc.IP)
	}

	var dst *net.IPNet
	if ip.To4() != nil {
		dst = &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}
	} else {
		dst = &net.IPNet{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       dst,
		Gw:        gw,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("add route via %s: %w", gw, err)
	}

	return nil
}

// disableChecksums disables rx/tx checksum offload via ethtool ioctl.
func disableChecksums(ifname string) error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil // Can't open socket — skip.
	}
	defer unix.Close(fd)

	// ETHTOOL_SRXCSUM — best-effort.
	_ = setEthtool(fd, ifname, 0x00000015, 0)

	// ETHTOOL_STXCSUM — required.
	if err := setEthtool(fd, ifname, 0x00000017, 0); err != nil {
		return fmt.Errorf("disable tx checksum on %s: %w", ifname, err)
	}

	return nil
}

func setEthtool(fd int, ifname string, cmd uint32, value uint32) error {
	req := struct {
		cmd  uint32
		data uint32
	}{cmd: cmd, data: value}

	ifr := [40]byte{}
	copy(ifr[:], ifname)
	*(*uintptr)(unsafe.Pointer(&ifr[16])) = uintptr(unsafe.Pointer(&req))

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.SIOCETHTOOL, uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		return errno
	}
	return nil
}
