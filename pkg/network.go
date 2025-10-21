package kitten

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"strconv"
	"strings"
	//"github.com/gohugoio/hugo/output"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func createVethPair(kittenID string) (string, string, error) {

	vethHost := "veth" + shortID(kittenID)
	vethContainer := "vethc" + shortID(kittenID)

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: vethHost,
		},
		PeerName: vethContainer,
	}

	err := netlink.LinkAdd(veth)
	if err != nil {
		return "", "", fmt.Errorf("failed to create veth pair: %w", err)
	}

	return vethHost, vethContainer, nil
}

func moveVethToNamespace(vethName string, pid int) error {
	link, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to find veth %s: %w", vethName, err)
	}

	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	netnsHandle, err := netns.GetFromPath(nsPath)

	if err != nil {
		return fmt.Errorf("failed to get netns: %w", err)
	}
	defer netnsHandle.Close()

	err = netlink.LinkSetNsFd(link, int(netnsHandle))
	if err != nil {
		return fmt.Errorf("failed to move veth to namespace: %w", err)
	}

	return nil
}

func renameVethInNamespace(pid int, oldName string, newName string) error {
	cmd := exec.Command("nsenter", "-t", strconv.Itoa(pid), "-n", "ip", "link", "set", oldName, "name", newName)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rename veth: %w (output: %s)", err, string(output))
	}

	return nil
}

func configureHostVeth(vethHost string, netConfig *NetworkConfig) error {

	link, err := netlink.LinkByName(vethHost)
	if err != nil {
		return fmt.Errorf("failed to find veth %s: %w", vethHost, err)
	}
	log.Printf("found veth: %s", vethHost)

	err = netlink.LinkSetUp(link)
	if err != nil {
		return fmt.Errorf("failed to bring up veth: %w", err)
	}
	log.Printf("brought up veth")

	if netConfig.Mode == "bridge" {
		log.Printf("stuck finding: %s, veth name: %s", netConfig.BridgeName, vethHost)
		bridge, err := netlink.LinkByName(netConfig.BridgeName)
		if err != nil {
			return fmt.Errorf("bridge does not exist: %w", err)
		}
		log.Printf("Found the bridge")
		err = netlink.LinkSetMaster(link, bridge.(*netlink.Bridge))
		if err != nil {
			return fmt.Errorf("failed to attach veth to bridge: %w", err)
		}

		gatewayAddr, err := netlink.ParseAddr(netConfig.GatewayIP + "/24")
		if err != nil {
			return fmt.Errorf("failed to parse gateway IP: %w", err)
		}

		addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("failed to list bridge IPs: %w", err)
		}
		hasGateway := false
		for _, addr := range addrs {
			if addr.IP.Equal(gatewayAddr.IP) {
				hasGateway = true
				break
			}
		}

		// Add gateway IP if not present
		if !hasGateway {
			log.Printf("Adding gateway IP %s to bridge", netConfig.GatewayIP)
			err = netlink.AddrAdd(bridge, gatewayAddr)
			if err != nil && !strings.Contains(err.Error(), "file exists") {
				return fmt.Errorf("failed to add gateway to bridge: %w", err)
			}
		} else {
			log.Printf("Bridge already has gateway IP")
		}
		// ADD THIS: Bring up the bridge
		err = netlink.LinkSetUp(bridge)
		if err != nil {
			return fmt.Errorf("failed to bring up bridge: %w", err)
		}
		log.Printf("Bridge is up")

		// Debug: Check final bridge state
		bridge, _ = netlink.LinkByName(netConfig.BridgeName)
		log.Printf("Bridge final state: %v, flags: %v", bridge.Attrs().OperState, bridge.Attrs().Flags)
	} else {
		log.Printf("Why are we not making a bridge")
		addr, err := netlink.ParseAddr(netConfig.GatewayIP + "/24")
		if err != nil {
			return fmt.Errorf("failed to parse gateway IP: %w", err)
		}
		err = netlink.AddrAdd(link, addr)
		if err != nil {
			return fmt.Errorf("failed to add IP to veth: %w", err)
		}
	}

	return nil
}

func deleteVethInterface(vethName string) error {
	link, _ := netlink.LinkByName(vethName)

	err := netlink.LinkDel(link)
	if err != nil {
		return fmt.Errorf("failed to delete veth: %w", err)
	}

	return nil
}

func addPortForward(pm PortMapping, containerIP string) error {

	rule := []string{
		"-t", "nat",
		"-A", "PREROUTING",
		"-p", pm.Protocol,
		"--dport", strconv.Itoa(pm.HostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort),
	}

	cmd := exec.Command("iptables", rule...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add DNAT rule: %w", err)
	}

	// Add MASQUERADE rule for return traffic
	rule = []string{
		"-t", "nat",
		"-A", "POSTROUTING",
		"-p", pm.Protocol,
		"-d", containerIP,
		"--dport", strconv.Itoa(pm.ContainerPort),
		"-j", "MASQUERADE",
	}

	cmd = exec.Command("iptables", rule...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add MASQUERADE rule: %w", err)
	}

	return nil
}

func removePortForward(pm PortMapping, containerIP string) error {
	rule := []string{
		"-t", "nat",
		"-D", "PREROUTING",
		"-p", pm.Protocol,
		"--dport", strconv.Itoa(pm.HostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort),
	}

	cmd := exec.Command("iptables", rule...)
	cmd.Run() // Ignore errors during cleanup

	// Remove MASQUERADE rule
	rule = []string{
		"-t", "nat",
		"-D", "POSTROUTING",
		"-p", pm.Protocol,
		"-d", containerIP,
		"--dport", strconv.Itoa(pm.ContainerPort),
		"-j", "MASQUERADE",
	}

	cmd = exec.Command("iptables", rule...)
	cmd.Run() // Ignore errors during cleanup

	return nil
}

func allocateIP(subnet string) string {
	// Parse subnet (e.g., "10.0.0.0/24")
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return ""
	}

	// Generate random IP in range (avoiding .0, .1, .255)
	ip := make(net.IP, len(ipnet.IP))
	copy(ip, ipnet.IP)
	ip[3] = byte(rand.Intn(253) + 2) // .2 to .254

	return ip.String()
}

func CreateBridge(bridgeName, subnet, gateway string) error {
	// Check if bridge already exists
	cmd := exec.Command("ip", "link", "show", bridgeName)
	if err := cmd.Run(); err == nil {
		// Bridge exists, just return
		return nil
	}

	// Create bridge
	cmd = exec.Command("ip", "link", "add", "name", bridgeName, "type", "bridge")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create bridge: %w (output: %s)", err, output)
	}

	// Set bridge IP address
	if gateway != "" {
		cmd = exec.Command("ip", "addr", "add", gateway+"/24", "dev", bridgeName)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Check if address already exists
			if !strings.Contains(string(output), "File exists") {
				return fmt.Errorf("failed to add IP to bridge: %w (output: %s)", err, output)
			}
		}
	}

	// Bring bridge up
	cmd = exec.Command("ip", "link", "set", bridgeName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w (output: %s)", err, output)
	}

	// Enable IP forwarding
	cmd = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	cmd.Run() // Ignore errors

	// Set up NAT for bridge
	if err := setupBridgeNAT(bridgeName); err != nil {
		return fmt.Errorf("failed to setup NAT: %w", err)
	}

	return nil
}

// DeleteBridge deletes a bridge network interface
func DeleteBridge(bridgeName string) error {
	// Bring bridge down
	cmd := exec.Command("ip", "link", "set", bridgeName, "down")
	cmd.Run() // Ignore errors

	// Delete bridge
	cmd = exec.Command("ip", "link", "delete", bridgeName, "type", "bridge")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore "Cannot find device" errors
		if !strings.Contains(string(output), "Cannot find device") {
			return fmt.Errorf("failed to delete bridge: %w (output: %s)", err, output)
		}
	}

	// Clean up NAT rules
	cleanupBridgeNAT(bridgeName)

	return nil
}

// setupBridgeNAT sets up NAT for the bridge
func setupBridgeNAT(bridgeName string) error {
	// Add MASQUERADE rule for outbound traffic
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.0.0.0/24", "-j", "MASQUERADE")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if rule already exists
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add MASQUERADE rule: %w (output: %s)", err, output)
		}
	}

	// Allow forwarding
	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add FORWARD rule: %w (output: %s)", err, output)
		}
	}

	cmd = exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add FORWARD rule: %w (output: %s)", err, output)
		}
	}

	return nil
}

// cleanupBridgeNAT removes NAT rules for the bridge
func cleanupBridgeNAT(bridgeName string) {
	// Remove MASQUERADE rule
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", "10.0.0.0/24", "-j", "MASQUERADE")
	cmd.Run() // Ignore errors

	// Remove FORWARD rules
	cmd = exec.Command("iptables", "-D", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	cmd.Run()

	cmd = exec.Command("iptables", "-D", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	cmd.Run()
}
