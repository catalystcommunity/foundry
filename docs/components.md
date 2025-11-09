# Components

Foundry manages the following core components for your infrastructure stack.

## OpenBAO

**Purpose**: Secrets management and secure storage

OpenBAO stores sensitive data including:
- API keys (PowerDNS, TrueNAS)
- SSH keys
- Kubernetes tokens
- Service credentials

**Deployment**: Container on infrastructure host
**Default Port**: 8200

## PowerDNS

**Purpose**: Authoritative DNS server with API

PowerDNS provides:
- Infrastructure zone (openbao, dns, zot, truenas, k8s nodes)
- Kubernetes zone (wildcard for ingress)
- Split-horizon DNS support
- HTTP API for dynamic record management

**Deployment**: Container on infrastructure host
**Default Ports**: 53 (DNS), 8081 (API)

## Zot

**Purpose**: OCI registry for container images

Zot provides:
- Private container registry
- Pull-through cache for Docker Hub
- Optional TrueNAS backend storage
- Kubernetes image source

**Deployment**: Container on infrastructure host
**Default Port**: 5000

## K3s

**Purpose**: Lightweight Kubernetes distribution

K3s provides:
- Kubernetes cluster (control plane + workers)
- kube-vip for HA control plane VIP
- Pre-configured with Zot registry
- DNS integration for service discovery

**Deployment**: Native install on cluster nodes
**Default Port**: 6443 (API server)

### kube-vip

**Purpose**: Virtual IP for HA control plane

Provides a single stable IP for the Kubernetes API server across multiple control plane nodes.

## Contour

**Purpose**: Ingress controller for Kubernetes

Contour provides:
- HTTP/HTTPS ingress routing
- TLS termination
- Envoy-based proxy

**Deployment**: Helm chart in Kubernetes

## cert-manager

**Purpose**: Automatic TLS certificate management

cert-manager provides:
- Certificate issuance and renewal
- Let's Encrypt integration
- Internal CA support

**Deployment**: Helm chart in Kubernetes

## TrueNAS (Optional)

**Purpose**: Network storage backend

TrueNAS provides:
- NFS storage for Zot registry
- Kubernetes persistent volumes
- Dataset and share management

**Integration**: API client for remote management
