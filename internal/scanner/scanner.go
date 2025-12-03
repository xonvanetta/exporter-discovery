package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const ExporterExporterPort = "9999"

type Target struct {
	IP       string
	Hostname string
	Subnet   string
	Module   string
}

type Scanner struct {
	workers int
	client  *http.Client
}

func New(workers int) *Scanner {
	return &Scanner{
		workers: workers,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (s *Scanner) ScanNetworks(ctx context.Context, networks []string) map[string][]Target {
	jobs := make(chan func(context.Context))
	results := make(chan Target, s.workers)

	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx, jobs)
		}()
	}

	go s.submitJobs(ctx, networks, jobs, results)

	targets := make(map[string][]Target)
	var resultsWg sync.WaitGroup
	resultsWg.Add(1)
	go func() {
		for target := range results {
			targets[target.Module] = append(targets[target.Module], target)
		}
		resultsWg.Done()
	}()

	wg.Wait()
	close(results)
	resultsWg.Wait()

	return targets
}

func (s *Scanner) worker(ctx context.Context, jobs <-chan func(context.Context)) {
	for job := range jobs {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		job(ctx)
		cancel()
	}
}

func (s *Scanner) submitJobs(ctx context.Context, networks []string, jobs chan<- func(context.Context), results chan<- Target) {
	defer close(jobs)
	for _, network := range networks {
		network = strings.TrimSpace(network)
		if network == "" {
			continue
		}
		networkIP, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			continue
		}

		for ip := networkIP.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			jobs <- s.createScanJob(ip.String(), network, results)
		}
	}
}

func (s *Scanner) createScanJob(ipStr, subnet string, results chan<- Target) func(context.Context) {
	return func(ctx context.Context) {
		modules, err := s.checkExporterExporter(ctx, ipStr)
		if err != nil {
			return
		}

		addrs, err := net.DefaultResolver.LookupAddr(ctx, ipStr)
		if err != nil || len(addrs) == 0 {
			return
		}

		hostname := strings.TrimRight(addrs[0], ".")

		for _, module := range modules {
			results <- Target{
				IP:       ipStr,
				Hostname: hostname,
				Subnet:   subnet,
				Module:   module,
			}
		}
	}
}

func (s *Scanner) checkExporterExporter(ctx context.Context, host string) ([]string, error) {
	url := fmt.Sprintf("http://%s", net.JoinHostPort(host, ExporterExporterPort))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var modules map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&modules); err != nil {
		return nil, err
	}

	result := make([]string, 0, len(modules))
	for name := range modules {
		result = append(result, name)
	}

	return result, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
