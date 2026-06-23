// Package charts embeds the Helm charts that foundry installs into the cluster,
// so the CLI can deploy them without any external chart repository.
package charts

import "embed"

// GatewayControllerChart holds the foundry-gateway-controller Helm chart. The
// "all:" prefix is required so go:embed includes templates/_helpers.tpl and the
// .helmignore (files beginning with "_" or "." are excluded by default).
//
//go:embed all:foundry-gateway-controller
var GatewayControllerChart embed.FS

// GatewayControllerChartDir is the path of the chart within the embedded FS.
const GatewayControllerChartDir = "foundry-gateway-controller"
