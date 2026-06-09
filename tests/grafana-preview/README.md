# Local Grafana Dashboard Preview

View the ENO Grafana dashboard locally with mock metric data.

## Prerequisites

- Docker and docker-compose (or podman-compose)

## Usage

```bash
cd tests/grafana-preview
docker compose up -d      # or: podman-compose up -d
```

Then open:
- **Grafana**: http://localhost:3000 (auto-login, no password needed)
- **Prometheus**: http://localhost:9090 (to inspect raw metrics)

The dashboard is auto-provisioned and should appear on the home page. If not,
navigate to Dashboards and look for "Ephemeral Namespace Operator Metrics".

## How it works

- **mock-metrics.py** — Python script that exposes fake Prometheus metrics on
  port 8000, matching every metric the dashboard queries. Values update every
  5 seconds so timeseries panels show realistic movement.
- **Prometheus** scrapes the mock exporter every 5s.
- **Grafana** is provisioned with Prometheus as the default datasource and the
  dashboard JSON extracted from the production ConfigMap.

## Iterating on the dashboard

1. Edit panels in the Grafana UI at http://localhost:3000
2. When happy, export the dashboard JSON (Dashboard settings > JSON Model)
3. Replace the JSON in `dashboards/ephemeral-ns-operator-dashboard.yaml` (inside
   the ConfigMap `data` field)

## Tear down

```bash
docker compose down
```
