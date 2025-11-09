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

Detect MAC addresses for DHCP reservations:
```bash
foundry network detect-macs
```

Validate network configuration:
```bash
foundry network validate
```

### 2. Setup Wizard

Run the interactive setup wizard:
```bash
foundry setup
```

The wizard will guide you through:
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
foundry component install powerdns
foundry component install zot
```

Check component status:
```bash
foundry component status openbao
```

## Troubleshooting

View component logs:
```bash
docker logs foundry-openbao
docker logs foundry-powerdns
docker logs foundry-zot
```

Test DNS resolution:
```bash
foundry dns test <zone>
```

For detailed component configuration, see [components.md](./components.md).
