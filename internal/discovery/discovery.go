// Package discovery finds devices on the local network without root
// privileges: it wakes the ARP cache, sweeps for TCP resets, reads the
// native ARP table, then probes and fingerprints TCP services.
package discovery

import (
	"context"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"wisee/internal/store"
)

// Config controls one scanner.
type Config struct {
	CIDRs       []string      // explicit IPv4 CIDRs; empty means auto-detect
	ExtraPorts  []int         // extra TCP ports to probe
	Timeout     time.Duration // per-connection timeout
	Concurrency int           // hosts probed in parallel
}

// Scanner runs network scans.
type Scanner struct {
	cfg   Config
	ports []int
}

// New builds a Scanner from the config.
func New(cfg Config) *Scanner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 350 * time.Millisecond
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 16
	}
	ports := append([]int{}, DefaultPorts()...)
	ports = append(ports, cfg.ExtraPorts...)
	return &Scanner{cfg: cfg, ports: dedupePorts(ports)}
}

// Scan performs one full sweep and returns one observation per live device.
func (s *Scanner) Scan(ctx context.Context) ([]store.Observation, error) {
	targets, err := localTargets(s.cfg.CIDRs)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		log.Println("discovery: no usable IPv4 network found")
		return nil, nil
	}

	wakeARP(targets)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(250 * time.Millisecond):
	}

	alive := sweepLiveness(targets, s.cfg.Timeout, s.cfg.Concurrency)
	for ip, mac := range readARP() {
		if _, ok := alive[ip]; !ok {
			alive[ip] = mac
		}
	}
	gateways := defaultGateways()

	hostname, _ := os.Hostname()
	var mu sync.Mutex
	var out []store.Observation

	seenSelf := map[string]bool{}
	for _, t := range targets {
		for _, ip := range t.ifaceIPs {
			seenSelf[ip] = true
		}
	}

	sem := make(chan struct{}, s.cfg.Concurrency)
	var wg sync.WaitGroup
	for ip, mac := range alive {
		ip, mac := ip, mac
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			obs := store.Observation{
				IP:     ip,
				MAC:    mac,
				Vendor: Vendor(mac),
			}
			if seenSelf[ip] {
				obs.Self = true
				obs.Name = hostname
			} else if name := lookupName(ctx, ip); name != "" {
				obs.Name = name
			}
			obs.Services = probeHost(ctx, ip, s.ports, s.cfg.Timeout)
			ports := make([]int, 0, len(obs.Services))
			for _, svc := range obs.Services {
				ports = append(ports, svc.Port)
			}
			obs.Kind = Classify(obs.Vendor, obs.Name, ip, gateways[ip], obs.Self, ports)

			mu.Lock()
			out = append(out, obs)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out, nil
}

func lookupName(ctx context.Context, ip string) string {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return names[0]
}

func dedupePorts(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range in {
		if p > 0 && p < 65536 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
