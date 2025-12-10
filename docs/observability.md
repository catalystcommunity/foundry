# Observability

Foundry deploys a complete observability stack for monitoring, logging, and alerting.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  Grafana (Dashboards)                                               │
│  - Visualize metrics and logs                                       │
│  - Pre-configured data sources                                      │
│  - Default dashboards for cluster health                            │
└─────────────────────────────────────────────────────────────────────┘
        │                              │
        ▼                              ▼
┌───────────────────────┐    ┌────────────────────────┐
│  Prometheus           │    │  Loki                  │
│  - Metrics collection │    │  - Log aggregation     │
│  - Alerting rules     │    │  - S3 backend storage  │
│  - ServiceMonitors    │    │  - Promtail collection │
└───────────────────────┘    └────────────────────────┘
        │                              │
        ▼                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                                  │
│  - Node metrics (node-exporter)                                     │
│  - Container metrics (kube-state-metrics)                           │
│  - Application logs (all pods)                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### Prometheus

Prometheus collects and stores time-series metrics from the cluster.

**Features:**
- Automatic service discovery via ServiceMonitors
- Persistent storage via Longhorn PVCs
- Built-in alerting with Alertmanager
- kube-state-metrics for Kubernetes object metrics
- node-exporter for host-level metrics

**Configuration:**
```yaml
prometheus:
  retention_days: 15
  retention_size: 10GB
  storage_size: 20Gi
  scrape_interval: 30s
  alertmanager_enabled: true
```

**Endpoints:**
- Prometheus: `http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090`
- Alertmanager: `http://kube-prometheus-stack-alertmanager.monitoring.svc.cluster.local:9093`

### Loki

Loki aggregates logs from all pods in the cluster.

**Features:**
- S3-compatible backend (SeaweedFS)
- Promtail for log collection
- Label-based log querying
- Configurable retention

**Configuration:**
```yaml
loki:
  retention_days: 30
  storage_backend: s3
  s3_endpoint: http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333
  s3_bucket: loki
```

**Endpoint:**
- `http://loki-gateway.loki.svc.cluster.local:80`

### Grafana

Grafana provides visualization dashboards for metrics and logs.

**Features:**
- Pre-configured Prometheus and Loki data sources
- Default dashboards for cluster monitoring
- Sidecar for automatic dashboard discovery
- Persistent storage for custom dashboards

**Configuration:**
```yaml
grafana:
  admin_user: admin
  storage_size: 5Gi
  ingress_enabled: true
  ingress_host: grafana.example.local
```

**Default Data Sources:**
- Prometheus (metrics)
- Loki (logs)

## ServiceMonitors

ServiceMonitors tell Prometheus which services to scrape for metrics. Foundry automatically creates ServiceMonitors for core components:

| Component | ServiceMonitor | Metrics Port | Path |
|-----------|----------------|--------------|------|
| Contour Envoy | contour-envoy | 8002 | /stats/prometheus |
| Contour Controller | contour-controller | metrics | /metrics |
| SeaweedFS Master | seaweedfs-master | 9333 | /metrics |
| SeaweedFS Volume | seaweedfs-volume | 8080 | /metrics |
| SeaweedFS Filer | seaweedfs-filer | 8888 | /metrics |
| Loki | loki | http-metrics | /metrics |
| Longhorn | longhorn-manager | manager | /metrics |
| Cert-Manager | cert-manager | http-metrics | /metrics |
| External-DNS | external-dns | http | /metrics |

### Creating Custom ServiceMonitors

Add a ServiceMonitor for your own applications:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-app
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: my-app
  endpoints:
    - port: metrics
      path: /metrics
      interval: 30s
```

## Commands

### View Metrics

```bash
# List available metrics targets
foundry metrics targets

# Query Prometheus metrics
foundry metrics "up"

# List all metric names
foundry metrics list
```

### View Logs

```bash
# Stream logs from a pod
foundry logs my-pod

# Stream logs with label filter
foundry logs -l app=nginx

# Query historical logs
foundry logs --query '{namespace="default"}'
```

### Access Dashboards

```bash
# Get Grafana URL
foundry dashboard url

# Open Grafana in browser
foundry dashboard open
```

## Default Dashboards

Foundry provisions default dashboards for:

- **Cluster Overview**: Node health, resource usage, pod status
- **Kubernetes Resources**: CPU, memory, network by namespace/pod
- **Node Exporter**: Host-level metrics (disk, CPU, memory, network)
- **Longhorn**: Storage capacity, volume health, IOPS

## Alerting

Alertmanager handles alert routing and notification.

**Default Alert Rules:**
- High CPU usage (>80% for 5 minutes)
- High memory usage (>85% for 5 minutes)
- Pod crash loops
- Node not ready
- PVC usage >85%
- Certificate expiring soon

**Configure Alert Receivers:**

Alert routing is configured via Alertmanager. Edit the kube-prometheus-stack Helm values to add receivers:

```yaml
alertmanager:
  config:
    receivers:
      - name: 'slack'
        slack_configs:
          - api_url: 'https://hooks.slack.com/...'
            channel: '#alerts'
```

## Troubleshooting

### Check Prometheus Status

```bash
kubectl -n monitoring get pods -l app.kubernetes.io/name=prometheus
kubectl -n monitoring logs -l app.kubernetes.io/name=prometheus
```

### Check Loki Status

```bash
kubectl -n loki get pods
kubectl -n loki logs -l app.kubernetes.io/name=loki
```

### Check Grafana Status

```bash
kubectl -n grafana get pods
kubectl -n grafana logs -l app.kubernetes.io/name=grafana
```

### Verify ServiceMonitors

```bash
# List all ServiceMonitors
kubectl get servicemonitors -A

# Check Prometheus targets
kubectl -n monitoring port-forward svc/kube-prometheus-stack-prometheus 9090:9090
# Then visit http://localhost:9090/targets
```

### Common Issues

**Metrics not appearing:**
1. Verify ServiceMonitor exists: `kubectl get servicemonitor -n monitoring`
2. Check target status in Prometheus UI
3. Verify service has correct labels matching ServiceMonitor selector

**Logs not appearing:**
1. Check Promtail is running: `kubectl -n loki get pods -l app.kubernetes.io/name=promtail`
2. Verify Promtail config: `kubectl -n loki logs -l app.kubernetes.io/name=promtail`
3. Check Loki ingester health: `kubectl -n loki logs -l app.kubernetes.io/component=single-binary`

**Grafana can't connect to data sources:**
1. Verify data source URLs are correct
2. Check network policies allow traffic
3. Test connectivity from Grafana pod: `kubectl exec -it grafana-xxx -- curl prometheus-url:9090/api/v1/status/config`
