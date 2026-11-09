package discovery

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// readARP returns the kernel ARP table as ip → MAC (uppercase, colon-separated).
func readARP() map[string]string {
	switch runtime.GOOS {
	case "linux":
		return readARPLinux()
	default:
		return readARPCommand()
	}
}

func readARPLinux() map[string]string {
	out := map[string]string{}
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return readARPCommand()
	}
	defer f.Close()
	parseARPLinux(bufio.NewScanner(f), out)
	return out
}

func parseARPLinux(sc *bufio.Scanner, out map[string]string) {
	first := true
	for sc.Scan() {
		if first { // header line
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		ip, mac := fields[0], normalizeMAC(fields[3])
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		out[ip] = mac
	}
}

var arpLineRe = regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\)\s+at\s+([0-9a-fA-F:-]+)|^\s*(\d+\.\d+\.\d+\.\d+)\s+([0-9a-fA-F-]{17})`)

// readARPCommand parses `arp -an` (macOS, BSD) or `arp -a` (Windows) output.
func readARPCommand() map[string]string {
	out := map[string]string{}
	args := []string{"-a"}
	if runtime.GOOS != "windows" {
		args = []string{"-an"}
	}
	cmd := exec.Command("arp", args...)
	b, err := cmd.Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		m := arpLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip, mac := m[1], m[2]
		if ip == "" {
			ip, mac = m[3], m[4]
		}
		mac = normalizeMAC(mac)
		if mac == "" || mac == "00:00:00:00:00:00" || strings.HasPrefix(mac, "FF:FF") {
			continue
		}
		out[ip] = mac
	}
	return out
}

// normalizeMAC converts any common MAC spelling to uppercase colon-separated.
func normalizeMAC(mac string) string {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	mac = strings.ReplaceAll(mac, "-", ":")
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) > 2 {
			return ""
		}
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}
