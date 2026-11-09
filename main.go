// Wi-See is a small web app that draws the devices on your home network as a
// map of bubbles. Live devices pulse; devices from earlier scans fade and
// keep their last-seen time.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wisee/internal/demo"
	"wisee/internal/discovery"
	"wisee/internal/store"
	"wisee/internal/web"
)

var version = "dev"

func main() {
	var (
		listen      = flag.String("listen", envOr("WISEE_LISTEN", "127.0.0.1:8090"), "HTTP listen address")
		dbPath      = flag.String("db", envOr("WISEE_DB", "wisee.db"), "SQLite database path")
		interval    = flag.Duration("interval", 60*time.Second, "time between scans (min 5s)")
		cidrs       = flag.String("cidr", "", "comma-separated IPv4 CIDRs to scan (default: auto-detect)")
		extraPorts  = flag.String("ports", "", "extra TCP ports to probe, e.g. 8000,9000-9010")
		timeout     = flag.Duration("timeout", 350*time.Millisecond, "per-connection probe timeout")
		concurrency = flag.Int("concurrency", 16, "hosts probed in parallel")
		demoMode    = flag.Bool("demo", false, "serve sample data without scanning")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("wisee", version)
		return
	}
	if *interval < 5*time.Second {
		*interval = 5 * time.Second
	}

	var cidrList []string
	if *cidrs != "" {
		for _, c := range splitComma(*cidrs) {
			cidrList = append(cidrList, c)
		}
	}
	portList, err := discovery.ParsePortsSpec(*extraPorts)
	if err != nil {
		log.Fatalf("-ports: %v", err)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open %s: %v", *dbPath, err)
	}
	defer st.Close()

	srv, err := web.New(st, version)
	if err != nil {
		log.Fatalf("web: %v", err)
	}

	scanner := discovery.New(discovery.Config{
		CIDRs:       cidrList,
		ExtraPorts:  portList,
		Timeout:     *timeout,
		Concurrency: *concurrency,
	})
	srv.RunScan = func() {
		runScan(scanner, st)
		srv.MarkScanned()
	}

	if *demoMode {
		if err := demo.Seed(st); err != nil {
			log.Fatalf("demo: %v", err)
		}
		srv.RunScan = nil
		log.Println("demo mode: sample network loaded, scanning disabled")
	} else {
		go scanLoop(scanner, st, srv, *interval)
	}

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("wisee %s listening on http://%s", version, *listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func scanLoop(scanner *discovery.Scanner, st *store.Store, srv *web.Server, interval time.Duration) {
	runScan(scanner, st)
	srv.MarkScanned()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		runScan(scanner, st)
		srv.MarkScanned()
	}
}

func runScan(scanner *discovery.Scanner, st *store.Store) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	obs, err := scanner.Scan(ctx)
	if err != nil {
		log.Printf("scan: %v", err)
		return
	}
	if err := st.ApplyScan(obs, time.Now()); err != nil {
		log.Printf("store: %v", err)
		return
	}
	log.Printf("scan complete: %d devices seen in %s", len(obs), time.Since(start).Round(time.Millisecond))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
