package discovery

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// target is one local IPv4 network to scan, with the interface it belongs to.
type target struct {
	ipnet    *net.IPNet
	ifaceIPs []string // our own addresses on this interface
}

// virtualPrefixes names interface prefixes that never carry home devices.
var virtualPrefixes = []string{
	"lo", "docker", "veth", "br-", "virbr", "tailscale", "ts-",
	"tun", "tap", "wg", "utun", "vmnet", "vboxnet", "anpi", "awdl",
	"llw", "bridge", "zt", "ham", "podman", "cni", "flannel",
	"vethernet", "hyper-v", "wsl", // Windows (Hyper-V/WSL friendly names)
}

// localTargets enumerates the IPv4 networks of real LAN interfaces, capped to
// a /24 each, or parses the explicit CIDRs when provided.
func localTargets(cidrs []string) ([]target, error) {
	if len(cidrs) > 0 {
		var out []target
		for _, c := range cidrs {
			_, ipnet, err := net.ParseCIDR(strings.TrimSpace(c))
			if err != nil {
				return nil, fmt.Errorf("cidr %q: %w", c, err)
			}
			ipnet = cap24(ipnet)
			if size := networkSize(ipnet); size > 1024 {
				return nil, fmt.Errorf("cidr %q too large (%d addresses), max is a /22", c, size)
			}
			out = append(out, target{ipnet: ipnet})
		}
		return out, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []target
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagPointToPoint != 0 || isVirtual(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			out = append(out, target{
				ipnet:    cap24(&net.IPNet{IP: ip4, Mask: ipnet.Mask}),
				ifaceIPs: []string{ip4.String()},
			})
		}
	}
	return out, nil
}

func isVirtual(name string) bool {
	name = strings.ToLower(name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// cap24 shrinks networks larger than a /24 to the local /24.
func cap24(ipnet *net.IPNet) *net.IPNet {
	if ones, bits := ipnet.Mask.Size(); bits == 32 && ones >= 24 {
		return ipnet
	}
	return &net.IPNet{
		IP:   ipnet.IP.To4().Mask(net.CIDRMask(24, 32)),
		Mask: net.CIDRMask(24, 32),
	}
}

func networkSize(ipnet *net.IPNet) int {
	ones, bits := ipnet.Mask.Size()
	return 1 << (bits - ones)
}

// hostIPs returns every usable host address in the network.
func hostIPs(ipnet *net.IPNet) []string {
	ip := ipnet.IP.To4()
	if ip == nil {
		return nil
	}
	ones, bits := ipnet.Mask.Size()
	count := 1 << (bits - ones)
	base := binary.BigEndian.Uint32(ip)
	var out []string
	for i := 1; i < count-1; i++ {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], base+uint32(i))
		out = append(out, net.IP(b[:]).String())
	}
	return out
}

// wakeARP sends one throwaway UDP datagram to every address so the kernel ARP
// cache learns who exists.
func wakeARP(targets []target) {
	sem := make(chan struct{}, 64)
	var wg sync.WaitGroup
	for _, t := range targets {
		for _, ip := range hostIPs(t.ipnet) {
			ip := ip
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				conn, err := net.DialTimeout("udp4", ip+":9", 200*time.Millisecond)
				if err != nil {
					return
				}
				conn.Write([]byte{0})
				conn.Close()
			}()
		}
	}
	wg.Wait()
}

// sweepLiveness connects to a usually-closed TCP port on every address: a
// fast RST ("connection refused") proves the host is alive even when it
// exposes no services at all.
func sweepLiveness(targets []target, timeout time.Duration, concurrency int) map[string]string {
	alive := map[string]string{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency*4)
	var wg sync.WaitGroup
	for _, t := range targets {
		for _, ip := range hostIPs(t.ipnet) {
			ip := ip
			// Our own addresses are alive by definition.
			ours := false
			for _, own := range t.ifaceIPs {
				if own == ip {
					ours = true
					break
				}
			}
			if ours {
				mu.Lock()
				alive[ip] = ""
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				conn, err := net.DialTimeout("tcp4", ip+":9", timeout)
				if err == nil {
					conn.Close()
					mu.Lock()
					alive[ip] = ""
					mu.Unlock()
					return
				}
				if strings.Contains(err.Error(), "refused") {
					mu.Lock()
					alive[ip] = ""
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	return alive
}

// defaultGateways returns the set of default gateway IPs (Linux only; empty
// elsewhere).
func defaultGateways() map[string]bool {
	out := map[string]bool{}
	if runtime.GOOS != "linux" {
		return out
	}
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		var gw uint32
		if _, err := fmt.Sscanf(fields[2], "%08X", &gw); err != nil {
			continue
		}
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], gw)
		out[net.IP(b[:]).String()] = true
	}
	return out
}
