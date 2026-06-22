# foundry-gateway-controller

Deploys the Foundry **gateway controller** into a cluster. The controller watches
`TLSRoute`/`TCPRoute` resources that target the Contour Gateway and opens the
matching L4 listener on the Gateway, the corresponding port on the Envoy service
(exposed on the cluster VIP via its `externalIPs`), and the matching ingress
rule on the Envoy `NetworkPolicy`.

This lets application charts declare their own ingress intent — a route with a
`parentRef` port — without an operator editing the central Contour/Gateway
config for every new port. See [docs/gateway-controller.md](../../../docs/gateway-controller.md)
for the full design.

## Install

```bash
helm install gateway-controller \
  deploy/charts/foundry-gateway-controller \
  --namespace foundry-system --create-namespace \
  --set image.tag=<version>
```

The image defaults to `containers.catalystsquad.com/public/catalystcommunity/foundry`
and the tag defaults to the chart `appVersion`.

## Values

| Key | Default | Description |
| --- | --- | --- |
| `replicaCount` | `1` | Controller replicas (the reconcile is idempotent). |
| `image.repository` | `containers.catalystsquad.com/public/catalystcommunity/foundry` | Image repository. |
| `image.tag` | `""` (chart `appVersion`) | Image tag. |
| `controller.gatewayName` | `contour` | Gateway to manage. |
| `controller.gatewayNamespace` | `projectcontour` | Gateway / Envoy namespace. |
| `controller.envoyService` | `contour-envoy` | Envoy service holding the VIP. |
| `controller.networkPolicy` | `contour-envoy` | Envoy NetworkPolicy (`""` to skip). |
| `controller.interval` | `15s` | Resync interval. |
| `controller.extraArgs` | `[]` | Extra args appended to the command. |
| `rbac.create` | `true` | Create the ClusterRole/Binding. |
| `serviceAccount.create` | `true` | Create the ServiceAccount. |

## RBAC

The controller needs a ClusterRole (routes are read cluster-wide):

- `gateway.networking.k8s.io`: `tlsroutes`, `tcproutes` — get/list/watch
- `gateway.networking.k8s.io`: `gateways` — get/list/watch/update/patch
- core `services` — get/list/watch/update/patch
- `networking.k8s.io` `networkpolicies` — get/list/watch/update/patch

The Gateway, Service and NetworkPolicy writes only ever touch the configured
`gatewayNamespace`; if you want to tighten this, split those three resources
into a namespaced Role and keep only the route read in the ClusterRole.
