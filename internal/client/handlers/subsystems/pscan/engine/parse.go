//go:build pscan

package engine

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaxHosts        = 1024
	MaxPorts        = 65535
	MaxWorkers      = 1024
	MaxRate         = 10000
	MaxScanDuration = 5 * time.Minute

	DefaultTimeout = 750 * time.Millisecond
	DefaultWorkers = 600
	DefaultPorts   = "21,22,23,25,53,80,88,135-139,161,389,443,445,623,636,860,873,902,10050,10051,10052,1090,1098,1099,1194,1195,1196,1197,1198,1199,1190,1433,1500,1521,1540,1541,1883,2001,2010,2022,2049,2222,2375,2376,2379,2380,2809,3050,3051,3128,3260,3268,3299,3300,3306,3389,3636,3690,4369,4444,4445,4786,4848,4899,4990,5432,5555,5556,5557,5558,5559,5666,5671,5672,5800,5900,5901,5985,6000,6001,6066,6129,6379,6666,7000-7004,7070,7071,7873,7990,8000-8003,8008,8009,8042,8080,8081,8082,8083,8088,8090,8181,8222,8291,8333,8383,8443,8500,8530,8531,8686,8879,8880,8883,8888,8983,9000,9000-9003,9012,9043,9060,9080,9081,9090,9091,9099,9380,9418,9443,9503,9800,9990,10000,10050,10123,10250,10255,10999,11002,11004,11006,11099,11111,15672,15674,15675,15692,25672,27017,27018,45000,45001,47001,47002,50000-50014,50500,61613,61614,61616"
)

type Config struct {
	Hosts       []netip.Addr
	Ports       []int
	Timeout     time.Duration
	Workers     int
	Rate        int
	JSON        bool
	MaxDuration time.Duration
}

func ParseArgs(args []string) (Config, error) {
	var (
		hostsRaw    string
		portsRaw    string
		timeoutRaw  string
		excludeRaw  string
		timeSeconds int
	)

	cfg := Config{
		Timeout:     DefaultTimeout,
		Workers:     DefaultWorkers,
		MaxDuration: MaxScanDuration,
	}

	fs := flag.NewFlagSet("pscan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&hostsRaw, "h", "", "comma-separated hosts, IPs or CIDRs")
	fs.StringVar(&hostsRaw, "ips", "", "comma-separated hosts, IPs or CIDRs")
	fs.StringVar(&portsRaw, "p", "", "comma-separated TCP ports, ranges or all")
	fs.StringVar(&portsRaw, "ports", "", "comma-separated TCP ports, ranges or all")
	fs.StringVar(&timeoutRaw, "timeout", DefaultTimeout.String(), "TCP connect timeout")
	fs.IntVar(&timeSeconds, "time", 0, "TCP connect timeout in seconds")
	fs.IntVar(&cfg.Workers, "workers", DefaultWorkers, "concurrent workers")
	fs.IntVar(&cfg.Workers, "t", DefaultWorkers, "concurrent workers")
	fs.IntVar(&cfg.Rate, "rate", 0, "maximum connection attempts per second")
	fs.StringVar(&excludeRaw, "exclude", "", "comma-separated IPs or CIDRs to skip")
	fs.StringVar(&excludeRaw, "hn", "", "comma-separated IPs or CIDRs to skip")
	fs.BoolVar(&cfg.JSON, "json", false, "emit JSONL results")
	if err := fs.Parse(normalizeFscanArgs(args)); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	if strings.TrimSpace(hostsRaw) == "" {
		return Config{}, fmt.Errorf("-h/--ips is required")
	}
	if strings.TrimSpace(portsRaw) == "" {
		portsRaw = DefaultPorts
	}

	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid --timeout: %w", err)
	}
	if timeout <= 0 {
		return Config{}, fmt.Errorf("--timeout must be positive")
	}
	cfg.Timeout = timeout
	if timeSeconds < 0 {
		return Config{}, fmt.Errorf("-time must be positive")
	}
	if timeSeconds > 0 {
		cfg.Timeout = time.Duration(timeSeconds) * time.Second
	}

	if cfg.Workers <= 0 {
		return Config{}, fmt.Errorf("--workers must be positive")
	}
	if cfg.Workers > MaxWorkers {
		return Config{}, fmt.Errorf("--workers exceeds maximum of %d", MaxWorkers)
	}
	if cfg.Rate < 0 {
		return Config{}, fmt.Errorf("--rate cannot be negative")
	}
	if cfg.Rate > MaxRate {
		return Config{}, fmt.Errorf("--rate exceeds maximum of %d", MaxRate)
	}

	exclusions, err := parseExclusions(excludeRaw)
	if err != nil {
		return Config{}, err
	}
	cfg.Hosts, err = parseHosts(hostsRaw, exclusions)
	if err != nil {
		return Config{}, err
	}
	if len(cfg.Hosts) == 0 {
		return Config{}, fmt.Errorf("no hosts remain after exclusions")
	}
	if len(cfg.Hosts) > MaxHosts {
		return Config{}, fmt.Errorf("host count %d exceeds maximum of %d", len(cfg.Hosts), MaxHosts)
	}

	cfg.Ports, err = parsePorts(portsRaw)
	if err != nil {
		return Config{}, err
	}
	if len(cfg.Ports) == 0 {
		return Config{}, fmt.Errorf("no ports selected")
	}
	if len(cfg.Ports) > MaxPorts {
		return Config{}, fmt.Errorf("port count %d exceeds maximum of %d", len(cfg.Ports), MaxPorts)
	}

	return cfg, nil
}

type exclusions struct {
	addrs    map[netip.Addr]struct{}
	prefixes []netip.Prefix
}

func parseExclusions(raw string) (exclusions, error) {
	result := exclusions{addrs: make(map[netip.Addr]struct{})}
	for _, item := range splitCSV(raw) {
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return exclusions{}, fmt.Errorf("invalid --exclude prefix %q: %w", item, err)
			}
			result.prefixes = append(result.prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return exclusions{}, fmt.Errorf("invalid --exclude IP %q: %w", item, err)
		}
		result.addrs[addr] = struct{}{}
	}
	return result, nil
}

func parseHosts(raw string, exclusions exclusions) ([]netip.Addr, error) {
	seen := make(map[netip.Addr]struct{})
	var hosts []netip.Addr
	for _, item := range splitCSV(raw) {
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("invalid --ips prefix %q: %w", item, err)
			}
			for addr := prefix.Masked().Addr(); prefix.Contains(addr); addr = addr.Next() {
				var err error
				hosts, err = appendHost(hosts, seen, addr, exclusions)
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			resolved, resolveErr := resolveHost(item)
			if resolveErr != nil {
				return nil, fmt.Errorf("invalid --ips host/IP %q: %w", item, resolveErr)
			}
			for _, resolvedAddr := range resolved {
				hosts, err = appendHost(hosts, seen, resolvedAddr, exclusions)
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		hosts, err = appendHost(hosts, seen, addr, exclusions)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].Compare(hosts[j]) < 0
	})
	return hosts, nil
}

func resolveHost(host string) ([]netip.Addr, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses resolved")
	}

	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addrs = append(addrs, addr.Unmap())
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no usable IP addresses resolved")
	}
	return addrs, nil
}

func appendHost(hosts []netip.Addr, seen map[netip.Addr]struct{}, addr netip.Addr, exclusions exclusions) ([]netip.Addr, error) {
	addr = addr.Unmap()
	if isExcluded(addr, exclusions) {
		return hosts, nil
	}
	if _, exists := seen[addr]; exists {
		return hosts, nil
	}
	hosts = append(hosts, addr)
	seen[addr] = struct{}{}
	if len(hosts) > MaxHosts {
		return nil, fmt.Errorf("host count exceeds maximum of %d", MaxHosts)
	}
	return hosts, nil
}

func isExcluded(addr netip.Addr, exclusions exclusions) bool {
	if _, ok := exclusions.addrs[addr]; ok {
		return true
	}
	for _, prefix := range exclusions.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parsePorts(raw string) ([]int, error) {
	seen := make(map[int]struct{})
	for _, item := range splitCSV(raw) {
		if strings.EqualFold(item, "all") {
			for port := 1; port <= MaxPorts; port++ {
				seen[port] = struct{}{}
			}
			continue
		}
		startText, endText, hasRange := strings.Cut(item, "-")
		start, err := parsePort(startText)
		if err != nil {
			return nil, err
		}
		end := start
		if hasRange {
			end, err = parsePort(endText)
			if err != nil {
				return nil, err
			}
			if end < start {
				return nil, fmt.Errorf("invalid port range %q", item)
			}
		}
		for port := start; port <= end; port++ {
			seen[port] = struct{}{}
			if len(seen) > MaxPorts {
				return nil, fmt.Errorf("port count exceeds maximum of %d", MaxPorts)
			}
		}
	}
	return sortPortsByScanPriority(seen), nil
}

func sortPortsByScanPriority(seen map[int]struct{}) []int {
	ports := make([]int, 0, len(seen))
	used := make(map[int]struct{}, len(priorityWebPorts))
	for _, port := range priorityWebPorts {
		if _, ok := seen[port]; ok {
			ports = append(ports, port)
			used[port] = struct{}{}
		}
	}

	remaining := make([]int, 0, len(seen)-len(ports))
	for port := range seen {
		if _, ok := used[port]; ok {
			continue
		}
		remaining = append(remaining, port)
	}
	sort.Ints(remaining)
	return append(ports, remaining...)
}

func parsePort(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d is outside 1-65535", port)
	}
	return port, nil
}

func normalizeFscanArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-p") && len(arg) > 2 && arg[2] != '-' && arg[2] != '=' {
			out = append(out, "-p", arg[2:])
			continue
		}
		if strings.HasPrefix(arg, "-h") && len(arg) > 2 && arg[2] != '-' && arg[2] != '=' && arg != "-help" {
			out = append(out, "-h", arg[2:])
			continue
		}
		out = append(out, arg)
	}
	return out
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
