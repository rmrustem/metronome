# Metronome

Metronome is a lightweight monitoring tool designed to perform periodic HTTP and TCP probes against specified targets. It exports metrics about probe status, latency, and TLS certificate expiration, allowing for easy integration with Prometheus and Grafana.

## Features

- Supports HTTP/HTTPS and TCP probes
- TLS certificate expiration as Prometheus metric
- Local file or remote URL configuration
- Automatic configuration reloading
- Customizable HTTP success criteria based on HTTP status codes, and keyword existence/non-existence in body content
- Prometheus metrics with custom labels
- Detailed HTTP latency tracking

## Metronome vs Prometheus Blackbox Exporter

Metronome is designed as a simpler, more lightweight alternative to the [Prometheus Blackbox Exporter](https://github.com/prometheus/blackbox_exporter). While the Blackbox Exporter is highly versatile and supports many protocols (ICMP, DNS, gRPC, etc.), it can be complex to configure due to its module-based architecture.

### Key differences
- Unlike the Blackbox Exporter, which performs probes synchronously when scraped by Prometheus, Metronome runs probes independently and asynchronously according to its own schedule. This ensures consistent probe intervals and makes metrics immediately available without adding latency to Prometheus scrapes.
- Metronome uses a straightforward per-probe configuration model, avoiding the multi-level module/target system that makes the Blackbox Exporter's configuration complex.
- Metronome has a highly focused codebase (under 1,000 lines of core Go logic), making it significantly easier to audit, understand, and extend for specific needs compared to the much larger and more modular Blackbox Exporter.

## Installation

To build Metronome from source, ensure you have Go 1.26 or later installed.

```bash
git clone https://github.com/rmrustem/metronome.git
cd metronome
go build -o metronome
```

## Usage

Metronome is configured primarily through environment variables and a configuration file containing probe definitions. By default, Metronome reads `config.yaml` from the current directory, so you can just run:

```bash
./metronome
```

To specify a different configuration file path, use the `METRONOME_CONFIG_PATH` environment variable:

```bash
METRONOME_CONFIG_PATH=/path/to/my-config.yaml ./metronome
```

By default, Metronome exports metrics on `:8080/metrics`.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `METRONOME_CONFIG_PATH` | Path to the local configuration file. | `config.yaml` |
| `METRONOME_CONFIG_URL` | URL to fetch the configuration from (overrides local file). | - |
| `METRONOME_CONFIG_URL_AUTH` | Value for the `Authorization` header when fetching remote config. | - |
| `METRONOME_CONFIG_RELOAD_INTERVAL` | How often to check for configuration changes (in seconds). Set to `0` to disable. | `60` |
| `METRONOME_PROBE_INTERVAL` | Default interval between probes if not specified (in seconds). | `30` |
| `METRONOME_HTTP_USER_AGENT` | Custom User-Agent header for HTTP probes. | `Metronome` |
| `METRONOME_HTTP_BODY_READ_BYTES` | Maximum number of bytes to read from an HTTP response body (e.g. for `contain` checks). | `102400` |
| `METRONOME_WEB_LISTEN` | Address and port for the Prometheus metrics server to listen on. | `:8080` |

### Configuration

Metronome uses a YAML configuration file (`config.yaml`) to define the probes.

```yaml
probes:
  - name: "godev_http"
    proto: "http"
    target: "https://go.dev"
    timeout: 5s
    labels:
      service: "go"
      region: "us-east-1"

  - name: "godev_tcp"
    proto: "tcp"
    target: "go.dev:443"
    tls: true
```

### Configuration Parameters

| Parameter | Description |
|-----------|-------------|
| `name`* | Unique name for the probe. |
| `proto`* | Protocol to use (`http` or `tcp`). |
| `target`* | The URL or host:port to probe. |
| `timeout` | Probe timeout (e.g., `5s`). |
| `labels` | Additional key-value pairs to add as Prometheus labels. |
| `success_codes` | Comma-separated list or ranges of HTTP status codes considered successful (e.g., `200-299,404`). |
| `contain` | String that must be present in the HTTP response body. |
| `not_contain` | String that must NOT be present in the HTTP response body. |
| `insecure_skip_verify` | If true, TLS certificate verification is skipped. |
| `tls` | If true, perform TLS handshake (only for TCP probes, HTTP uses URL scheme). |

## Exported Metrics

- `metronome_probe_status`: 1 for success, 0 for failure.
- `metronome_probe_latency_seconds`: Total round-trip time in seconds.
- `metronome_tls_expires`: Unix timestamp of the server certificate expiration.
- `metronome_http_latency_seconds`: HTTP latency by phase (dns, connect, tls, wait_for_response).
- `metronome_probe_requests_total`: Total number of requests for each probe.
- `metronome_probe_failure_reason`: Reason code for probe failure (0 for success).

### Failure Reason Codes

| Code | Description |
|------|-------------|
| `0` | Probe was successful. |
| `1001` | Failed to resolve the target hostname. |
| `1101` | Network connection timed out. |
| `1102` | Connection was refused by the target. |
| `1201` | Generic TLS handshake failure (including SNI issues). |
| `1202` | Target certificate has expired. |
| `1203` | Certificate signed by an unknown CA. |
| `1204` | Certificate does not match the target hostname. |
| `1205` | Other certificate validation errors. |
| `1300` | Could not construct the HTTP request. |
| `1301` | Response status code did not match success criteria. |
| `1302` | Failed to read the response body. |
| `1303` | `contain` string was not found in the response body. |
| `1304` | `not_contain` string was found in the response body. |

## Prometheus Alerting Rules

Below are examples of Prometheus alerting rules you can use with Metronome metrics.

```yaml
groups:
  - name: metronome_alerts
    rules:
      # Alert on probe failure
      - alert: ProbeFailed
        expr: metronome_probe_status == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Probe {{ $labels.name }} failed"
          description: >
            Probe {{ $labels.name }} against {{ $labels.target }} is failing.
            Failure code: {{ query "metronome_probe_failure_reason{name='{{ $labels.name }}'}" }}.

      # Alert on TLS certificate expiration (30 days)
      - alert: TLSCertExpiringSoon
        expr: metronome_tls_expires - time() < 30 * 24 * 3600
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "TLS certificate for {{ $labels.name }} expiring soon"
          description: "The TLS certificate for {{ $labels.target }} will expire in {{ $value | humanizeDuration }}."
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
