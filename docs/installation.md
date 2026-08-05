# Installation Guide

## Prerequisites

### Infrastructure Hosts

- Ubuntu 22.04+ or similar Linux distribution
- SSH access with sudo privileges
- Docker or Podman installed
- Static IP addresses or DHCP reservations
- Network access between all hosts

### Network Requirements

- Dedicated subnet for infrastructure components
- VIP address for Kubernetes control plane
- DNS delegation or ability to configure authoritative DNS

## Quick Start

### 1. Network Planning

Run the network planning wizard:

```bash
foundry network plan
```

Validate network configuration:

```bash
foundry network validate
```

### 2. Stack Installation

Install the stack:

```bash
foundry stack install
```

The installer guides you through:
- Network and DNS configuration
- Component selection
- Host assignment
- Stack installation

### 3. Verify Installation

Check stack status:

```bash
foundry stack status
```

Validate deployment:

```bash
foundry stack validate
```

## Component Installation

Install individual components:

```bash
foundry component install openbao
foundry component install dns
foundry component install zot
```

Check component status:

```bash
foundry component status openbao
```

## Troubleshooting

View the logs for a Kubernetes pod:

```bash
foundry logs <pod-name> --namespace <namespace>
```

Test DNS resolution:

```bash
foundry dns test <hostname>
```

For detailed component configuration, see [components.md](./components.md).
