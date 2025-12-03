package registry

import (
	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/component/certmanager"
	"github.com/catalystcommunity/foundry/v1/internal/component/contour"
	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/component/gatewayapi"
	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/minio"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/component/storage"
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
	// Storage provides PVC provisioning via local-path, NFS, or TrueNAS CSI
	storageComp := storage.NewComponent(nil, nil)
	if err := component.Register(storageComp); err != nil {
		return err
	}

	// Register MinIO - depends on storage for PVCs
	// MinIO provides S3-compatible object storage for Loki, Velero, etc.
	minioComp := minio.NewComponent(nil, nil)
	if err := component.Register(minioComp); err != nil {
		return err
	}

	return nil
}
