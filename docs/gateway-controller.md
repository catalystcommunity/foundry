# Gateway Controller

The gateway controller lets application charts open L4 (TCP/TLS) listeners on the
shared Contour Gateway — and on the cluster VIP — purely by declaring a route in
their own chart. No operator has to edit the central Contour configuration for
each new port.

## Why

Foundry installs Contour with a single, statically-provisioned Envoy that holds
the cluster VIP (via `externalIPs` on the `contour-envoy` Service). Opening a new
port on that shared data plane normally requires three coordinated changes that
only an operator can make:

1. add a listener to the `contour` Gateway,
2. add the port to the `contour-envoy` Service (so the VIP answers on it), and
3. allow the port through the Envoy `NetworkPolicy`.

Gateway API splits ownership on purpose — the operator owns the **Gateway**
(listeners/ports), and the app owns the **route** (hostname → service). The
controller automates the operator half: it derives the needed listeners from the
routes themselves, so an app team only writes a route.

> For UDP, Contour has no support — expose it with a plain Service that lists the
> VIP in `externalIPs`. See the "Alternatives" section.

## The model

An app declares intent with a `TLSRoute` or `TCPRoute` whose `parentRefs` target
the Contour Gateway. The listener port can be declared just once, on the
`backendRefs` (the natural place — it's the Service port); `parentRefs[].port` is
optional and **overrides** the backend port when set:

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TLSRoute
metadata:
  name: linkkeys
  namespace: linkkeys
spec:
  parentRefs:
    - name: contour
      namespace: projectcontour
      # port: 4987          # optional — overrides the backend port when present
  hostnames:
    - linkkeys.example.com  # routed by SNI (TLS passthrough)
  rules:
    - backendRefs:
        - name: linkkeys
          port: 4987        # the port the controller opens
```

The controller derives the listener `port` from `parentRefs[].port` when set,
otherwise from the route's `rules[].backendRefs[].port`; the protocol comes from
the route kind:

| Route kind | Listener protocol | Routing |
| --- | --- | --- |
| `TLSRoute` | `TLS` (Passthrough) | by **SNI** — TLS terminates at the backend |
| `TCPRoute` | `TCP` | by **port** only (no hostname) |

Multiple `TLSRoute`s can share one listener/port and are disambiguated by SNI, so
two domains on port 4987 each terminate TLS at their own backend.

## What it reconciles

For every desired `(port, protocol)`, the controller converges three resources in
the Gateway namespace:

1. **Gateway listener** — named `gw-<proto>-<port>` (e.g. `gw-tls-4987`), with
   `allowedRoutes` for the matching route kind and `from: All`.
2. **`contour-envoy` Service port** — `port` → `targetPort = port + 8000`. Contour
   binds Envoy to the listener port plus 8000 (80→8080, 443→8443, 4987→12987); the
   port is reachable on the VIP automatically through the existing `externalIPs`.
3. **Envoy `NetworkPolicy` ingress** — admits the remapped target port (the default
   policy only allows 8080/8443/8002).

It owns only what it creates: `gw-`prefixed listeners/ports, and NetworkPolicy
ports recorded in the `gateway.foundry.dev/managed-target-ports` annotation. The
built-in HTTP/HTTPS listeners and any operator-pinned static listeners are never
touched. Deleting a route prunes its port. Because it reconciles continuously, it
also re-applies itself after a `foundry stack install`/upgrade resets the Service.

Ports **80** and **443** are reserved for the built-in listeners and are refused.
A port requested as both TLS and TCP is skipped with a logged conflict. A route
that targets the Gateway but yields no listener port — neither a `parentRefs[].port`
nor a single `backendRefs[].port` (none set, or backends declaring different
ports) — is skipped and named in the resync log so it's clear what was ignored
and why, instead of vanishing silently.

## Running it

### As a CLI (local / ad hoc)

```bash
foundry gateway controller            # watch loop, 15s resync (Ctrl-C to stop)
foundry gateway controller --once     # single reconcile pass, then exit
```

Flags: `--gateway-name`, `--gateway-namespace`, `--envoy-service`,
`--network-policy` (empty to skip), `--interval`, `--kubeconfig`.

Client config resolves in order: `--kubeconfig` → in-cluster service account (when
running as a pod) → `~/.foundry/kubeconfig`.

### In-cluster (foundry component — recommended)

foundry ships the chart embedded in the binary and installs it as the
`gateway-controller` component (single-replica Deployment + ServiceAccount +
ClusterRole/Binding in `foundry-system`, pointing at the published image).

**As part of `foundry stack install` — opt-in.** The component is in the install
order but is **skipped unless explicitly enabled**, so a default stack install
never depends on the controller image being present. Enable it in the stack
config:

```yaml
components:
  gateway-controller:
    enabled: true
    # optional typed shortcuts:
    # image_tag: "0.2.0"
    # interval: "30s"
    # replica_count: 1
    # raw chart values overlay (values.yaml equivalent) — deep-merged on top of
    # the typed fields and the chart defaults; this wins:
    values:
      resources:
        limits:
          cpu: 200m
      podAnnotations:
        team: ingress
      controller:
        interval: 60s   # overrides just this leaf; siblings keep their defaults
```

Then `foundry stack install` installs (or upgrades) it; flip `enabled` back to
`false`/remove it and it's simply skipped on the next run.

The `values` map is passed through to the Helm chart exactly like a
`-f values.yaml`: anything the chart's `values.yaml` exposes (resources,
nodeSelector, tolerations, affinity, podAnnotations, securityContext,
`controller.extraArgs`, …) can be set there. It is deep-merged **on top of** the
typed fields, so `values` always wins and you can override a single nested leaf
without restating its siblings.

**On demand — any time:**

```bash
foundry component install gateway-controller
```

### In-cluster (Helm directly)

The same chart can be installed with helm (it lives in the module at
`v1/charts/foundry-gateway-controller`):

```bash
helm install gateway-controller \
  v1/charts/foundry-gateway-controller \
  --namespace foundry-system --create-namespace \
  --set image.tag=<version>
```

See [v1/charts/foundry-gateway-controller/README.md](../v1/charts/foundry-gateway-controller/README.md)
for all values.

CI builds and publishes both the image and the chart on merge to `main` via
reactorcide (see `.reactorcide/jobs/`); because the chart is embedded in the
binary, a chart change also produces a new image.

### RBAC

The controller reads routes cluster-wide and writes only to the Gateway namespace:

- `gateway.networking.k8s.io`: `tlsroutes`, `tcproutes` — get/list/watch
- `gateway.networking.k8s.io`: `gateways` — get/list/watch/update/patch
- core `services` — get/list/watch/update/patch
- `networking.k8s.io` `networkpolicies` — get/list/watch/update/patch

## Alternatives

- **Static operator-pinned listeners.** If a port should always be open regardless
  of routes, declare it once under `components.contour.listeners` in the stack
  config — each entry has `name`, `protocol` (`TCP`/`TLS`), `port`, and optional
  `tls_mode`/`hostname`/`certificate_ref`. Foundry opens it during
  `stack install` (Gateway listener + Envoy service port + NetworkPolicy). The
  controller leaves these untouched — the two coexist.
- **UDP.** Contour does not implement `UDPRoute`. Expose UDP with a plain Service
  that lists the VIP in `externalIPs` and a `protocol: UDP` port, owned by the
  app's chart. It bypasses the Gateway entirely.
