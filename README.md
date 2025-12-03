# exporter-discovery

Network scanner for Prometheus exporters with Kubernetes ScrapeConfig integration for kube-prometheus stack.

## What It Does

1. Scans CIDR ranges for hosts running exporter_exporter (port 9999)
2. Queries available modules from each exporter_exporter instance
3. Performs reverse DNS lookups for hostname resolution
4. Creates/updates one ScrapeConfig per module in Kubernetes
5. Merges new targets with existing ones (preserves targets not found in current scan)
6. Supports per-module scrape intervals and timeouts

## Purpose

Used within kube-prometheus stack to automatically discover and configure monitoring for physical/bare-metal hosts running exporter_exporter.

## Configuration

See `config.yaml.example` for full configuration options:

```yaml
networks:
  - "10.33.10.0/24"
  - "192.168.1.0/24"

interval: "60m"
namespace: "monitoring"
workers: 128

modules:
  - name: "ipmi"
    interval: "5m"
    timeout: "30s"
  - name: "node"
    interval: "1m"
    timeout: "10s"
```

Configuration fields:

- `networks`: List of CIDR ranges to scan (required)
- `interval`: How often to run discovery (default: 60m)
- `namespace`: Kubernetes namespace for ScrapeConfigs (default: monitoring)
- `workers`: Number of concurrent scanner goroutines (default: 128)
- `modules`: Per-module ScrapeConfig settings (optional)
  - `name`: Module name from exporter_exporter
  - `interval`: Prometheus scrape interval for this module
  - `timeout`: Prometheus scrape timeout for this module

## Deployment with Helm

### From GitHub OCI Registry

```bash
helm install exporter-discovery oci://ghcr.io/xonvanetta/charts/exporter-discovery \
  --version 1.0.0 \
  -n monitoring
```

Custom values:

```bash
helm install exporter-discovery oci://ghcr.io/xonvanetta/charts/exporter-discovery \
  --version 1.0.0 \
  -n monitoring \
  --set config.networks[0]="10.0.0.0/24"
```

Or with custom values file:

```bash
helm install exporter-discovery oci://ghcr.io/xonvanetta/charts/exporter-discovery \
  --version 1.0.0 \
  -n monitoring \
  -f values-prod.yaml
```

### From Local Chart

```bash
helm install exporter-discovery ./helm -n monitoring
```

Custom values:

```bash
helm install exporter-discovery ./helm -n monitoring \
  --set config.networks[0]="10.0.0.0/24" \
  --set image.tag=1.0.0
```

Or with custom values file:

```bash
helm install exporter-discovery ./helm -n monitoring -f values-prod.yaml
```

## Manual Deployment

### Using Makefile

Build and push Docker image:

```bash
TAG=1.0.0 make push
```

Package and push Helm chart:

```bash
TAG=1.0.0 make helm-push
```

Deploy with Helm:

```bash
TAG=1.0.0 make deploy
```

Dry run:

```bash
make dry
```

### Local Development

Build:

```bash
go build -o exporter-discovery ./cmd
```

Run locally:

```bash
./exporter-discovery -config config.yaml -workers 128
```

## GitHub Workflow

The project includes GitHub Actions workflows for CI/CD:

- **Build**: Runs on push/PR to main/master
- **Release**: Triggered by version tags

To create a release:

```bash
git tag -a 1.0.0 -m "Release 1.0.0"
git push --follow-tags
```

This will:
1. Run tests
2. Build and push Docker image to ghcr.io
3. Package and push Helm chart to ghcr.io
4. Create GitHub release with release notes and Helm chart package

## Architecture

```
Networks → Scanner (128 workers) → exporter_exporter:9999 → Modules
                                                              ↓
                                                    One ScrapeConfig per Module
```

Key components:

- `internal/config`: YAML configuration loading
- `internal/scanner`: Network scanning with goroutine worker pool
- `internal/k8s`: Kubernetes ScrapeConfig creation and updates
- `cmd/main.go`: Main application with ticker-based periodic scanning

## Output

Creates ScrapeConfigs named `exporter-discovery-{module}` with target labels:
- `host`: Hostname from reverse DNS lookup
- `module`: Module name
- `__metrics_path__`: Set to `/proxy`
- `__param_module`: Set to module name

Example ScrapeConfig created:

```yaml
apiVersion: monitoring.coreos.com/v1alpha1
kind: ScrapeConfig
metadata:
  name: exporter-discovery-ipmi
  namespace: monitoring
spec:
  scrapeInterval: 5m
  scrapeTimeout: 30s
  staticConfigs:
  - targets:
    - "10.33.10.15:9999"
    labels:
      host: "server01.example.com"
      module: "ipmi"
      __metrics_path__: "/proxy"
      __param_module: "ipmi"
  - targets:
    - "10.33.10.20:9999"
    labels:
      host: "server02.example.com"
      module: "ipmi"
      __metrics_path__: "/proxy"
      __param_module: "ipmi"
```

## Requirements

- Go 1.22+
- Kubernetes cluster with prometheus-operator
- RBAC permissions for ScrapeConfig resources
- Physical hosts with exporter_exporter running on port 9999

## TODO

- **Change ScrapeConfig structure from per-module to per-host**: Currently the application creates one ScrapeConfig resource per module (e.g., `exporter-discovery-ipmi`, `exporter-discovery-node`), with all hosts exposing that module grouped together. This should be refactored to create one ScrapeConfig per discovered host (e.g., `exporter-discovery-server01.example.com`), containing all modules available on that host. This would provide better isolation, easier debugging per-host, and more granular control over individual host configurations.

## License

See LICENSE file for details.
