package registry

import (
	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/component/certmanager"
	"github.com/catalystcommunity/foundry/v1/internal/component/contour"
	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/component/externaldns"
	"github.com/catalystcommunity/foundry/v1/internal/component/gatewayapi"
	"github.com/catalystcommunity/foundry/v1/internal/component/grafana"
	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/loki"
	"github.com/catalystcommunity/foundry/v1/internal/component/seaweedfs"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/component/prometheus"
	"github.com/catalystcommunity/foundry/v1/internal/component/storage"
	"github.com/catalystcommunity/foundry/v1/internal/component/velero"
	"github.com/catalystcommunity/foundry/v1/internal/component/zot"
)

// InitComponents registers all available components in the default registry.
// Components are registered with minimal dependencies - actual SSH connections
// and configuration are provided at installation time via ComponentConfig.
func InitComponents() error {
	// Register OpenBAO - no dependencies
	if err := component.Register(&openbao.Component{}); err != nil {
		return err
	}

	// Register DNS (PowerDNS) - depends on OpenBAO for API key storage
	dnsComp := dns.NewComponent()
	if err := component.Register(dnsComp); err != nil {
		return err
	}

	// Register Zot - depends on DNS and OpenBAO
	if err := component.Register(&zot.Component{}); err != nil {
		return err
	}

	// Register K3s - depends on OpenBAO, DNS, and Zot
	if err := component.Register(&k3s.Component{}); err != nil {
		return err
	}

	// Register Gateway API - depends on K3s
	// Gateway API CRDs are installed as a cluster-level feature, independent of ingress controllers
	gatewayAPIComp := gatewayapi.NewComponent(nil)
	if err := component.Register(gatewayAPIComp); err != nil {
		return err
	}

	// Register Contour - depends on K3s and Gateway API
	// Note: Contour requires Helm and K8s clients which are initialized at runtime
	contourComp := contour.NewComponent(nil, nil)
	if err := component.Register(contourComp); err != nil {
		return err
	}

	// Register cert-manager - depends on K3s
	// Note: cert-manager requires Helm and K8s clients which are initialized at runtime
	certManagerComp := certmanager.NewComponent(nil)
	if err := component.Register(certManagerComp); err != nil {
		return err
	}

	// Register storage - depends on K3s
	// Storage provides PVC provisioning via local-path, NFS, or Longhorn
	storageComp := storage.NewComponent(nil, nil)
	if err := component.Register(storageComp); err != nil {
		return err
	}

	// Register SeaweedFS - depends on storage for PVCs
	// SeaweedFS provides S3-compatible object storage for Loki, Velero, etc.
	seaweedfsComp := seaweedfs.NewComponent(nil, nil)
	if err := component.Register(seaweedfsComp); err != nil {
		return err
	}

	// Register Prometheus - depends on storage for PVCs
	// Prometheus provides metrics collection and alerting
	prometheusComp := prometheus.NewComponent(nil, nil)
	if err := component.Register(prometheusComp); err != nil {
		return err
	}

	// Register Loki - depends on storage and seaweedfs for log storage
	// Loki provides centralized log aggregation
	lokiComp := loki.NewComponent(nil, nil)
	if err := component.Register(lokiComp); err != nil {
		return err
	}

	// Register Grafana - depends on prometheus and loki for data sources
	// Grafana provides unified observability dashboards
	grafanaComp := grafana.NewComponent(nil, nil)
	if err := component.Register(grafanaComp); err != nil {
		return err
	}

	// Register External-DNS - depends on k3s
	// External-DNS automatically manages DNS records for Kubernetes resources
	externaldnsComp := externaldns.NewComponent(nil, nil)
	if err := component.Register(externaldnsComp); err != nil {
		return err
	}

	// Register Velero - depends on seaweedfs for backups
	// Velero provides cluster backup and restore capabilities
	veleroComp := velero.NewComponent(nil, nil)
	if err := component.Register(veleroComp); err != nil {
		return err
	}

	return nil
}
