package discovery

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisee/internal/store"
)

// portNames maps well-known TCP ports to service names.
var portNames = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns",
	80: "http", 110: "pop3", 139: "netbios", 143: "imap", 443: "https",
	445: "smb", 548: "afp", 554: "rtsp", 587: "smtp", 631: "ipp",
	873: "rsync", 993: "imaps", 995: "pop3s", 1883: "mqtt", 3000: "http-dev",
	3306: "mysql", 3389: "rdp", 5000: "upnp", 5353: "mdns", 5432: "postgres",
	5900: "vnc", 6379: "redis", 8008: "chromecast", 8009: "chromecast",
	8080: "http-alt", 8081: "http-alt", 8096: "jellyfin", 8123: "home-assistant",
	8443: "https-alt", 8883: "mqtts", 9000: "http-alt", 9090: "cockpit",
	9100: "pdl-print", 32400: "plex", 62078: "iphone-sync",
}

// httpPorts are worth an HTTP title grab.
var httpPorts = map[int]bool{
	80: true, 3000: true, 5000: true, 8008: true, 8080: true, 8081: true,
	8096: true, 8123: true, 9000: true, 9090: true, 32400: true,
}

// httpsPorts are worth a TLS handshake and an HTTPS title grab.
var httpsPorts = map[int]bool{443: true, 8443: true}

// DefaultPorts returns the curated probe list.
func DefaultPorts() []int {
	out := make([]int, 0, len(portNames))
	for p := range portNames {
		out = append(out, p)
	}
	return out
}

// ParsePortsSpec parses "8000,9000-9010" into a port list.
func ParsePortsSpec(spec string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			lo, err1 := strconv.Atoi(lo)
			hi, err2 := strconv.Atoi(hi)
			if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			for p := lo; p <= hi; p++ {
				out = append(out, p)
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		out = append(out, p)
	}
	return out, nil
}

// probeHost probes every port on one host concurrently and returns the open
// services, fingerprinted.
func probeHost(ctx context.Context, ip string, ports []int, timeout time.Duration) []store.ServiceObs {
	var mu sync.Mutex
	var out []store.ServiceObs
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, port := range ports {
		port := port
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			addr := net.JoinHostPort(ip, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp4", addr, timeout)
			if err != nil {
				return
			}
			conn.Close()
			name, title := fingerprint(ip, port, timeout)
			mu.Lock()
			out = append(out, store.ServiceObs{Port: port, Name: name, Title: title})
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// fingerprint names the service and grabs identifying details: HTTP titles,
// TLS certificate names and SSH banners.
func fingerprint(ip string, port int, timeout time.Duration) (name, title string) {
	name = portNames[port]
	if name == "" {
		name = "tcp"
	}
	switch {
	case httpPorts[port]:
		if t := httpTitle("http://"+net.JoinHostPort(ip, strconv.Itoa(port)), timeout*4, false); t != "" {
			title = t
		}
	case httpsPorts[port]:
		if t, cn := httpsTitle("https://"+net.JoinHostPort(ip, strconv.Itoa(port)), timeout*4); t != "" {
			title = t
		} else if cn != "" {
			title = cn
		}
	case port == 22:
		title = sshBanner(ip, port, timeout*2)
	}
	if len(title) > 120 {
		title = title[:120]
	}
	return name, title
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>\s*([^<]{1,200}?)\s*</title>`)

func httpTitle(url string, timeout time.Duration, insecure bool) string {
	t, _ := httpsGet(url, timeout, insecure)
	return t
}

func httpsTitle(url string, timeout time.Duration) (title, certName string) {
	return httpsGet(url, timeout, true)
}

func httpsGet(url string, timeout time.Duration, insecure bool) (title, certName string) {
	transport := &http.Transport{}
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // LAN fingerprinting
	}
	client := &http.Client{Timeout: timeout, Transport: transport}
	resp, err := client.Get(url)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		certName = resp.TLS.PeerCertificates[0].Subject.CommonName
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return "", certName
	}
	if m := titleRe.FindSubmatch(body); m != nil {
		return strings.TrimSpace(string(m[1])), certName
	}
	return "", certName
}

func sshBanner(ip string, port int, timeout time.Duration) string {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
	if err != nil {
		return ""
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 255)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(buf[:n]), "\n")
	return strings.TrimSpace(line)
}
