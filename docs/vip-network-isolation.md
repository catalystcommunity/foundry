# VIP network conversion

Flannel can select a floating VIP when the VIP and the node address share an
interface. An automatic K3s restart can cause this fault. The API server can
then fail to reach admission webhooks, even when their Pods are Ready.

Foundry puts API and service VIPs on `foundry-vip`, a Linux macvlan interface.
The node address stays on its original interface. Foundry sets `node-ip` and
`flannel-iface` in a managed K3s configuration file. A systemd startup command
creates the VIP interface before each K3s start. No extra VM network device is
necessary.

## Convert an existing cluster

Use the current Foundry build and the cluster configuration:

```sh
foundry --config ~/.foundry/catalyst.yaml cluster network --dry-run
foundry --config ~/.foundry/catalyst.yaml cluster network
```

The command checks all configured control planes. It prepares their interfaces
and startup files before it changes kube-vip. It then restarts one K3s server
at a time. During each restart, the existing guard excludes that server from
kube-vip. Foundry checks the node address, Flannel tunnel, and replacement
kube-vip Pod before it continues. A single-server cluster has a brief API
outage during its K3s restart.

`cluster init` and the stack upgrade path also run this conversion. Adding a
control plane prepares its interface before the node joins. A repeat run checks
the result and skips restarts on servers that already have the required state.
The command does not change Kyverno policies.

## Requirements and limits

- All existing control planes must be present in the Foundry host inventory.
- Hosts need systemd, Python 3, iproute2, and an Ethernet interface with ARP.
- The current conversion supports IPv4 and the Flannel VXLAN backend.
- A custom K3s configuration path or conflicting network settings stop the
  preflight. Resolve the conflict before conversion.
- The network must allow traffic from the macvlan MAC addresses.
- For Tailscale, keep the VIP route directed to its active holder. See
  [Tailscale integration](tailscale-integration.md).

Foundry refuses to replace an unmanaged interface or file at a managed path.
It stores the prior DaemonSet update strategy in an annotation during
conversion. If a node restart fails, the restart guard keeps kube-vip off that
node. Verify K3s and Flannel, then recover the guard from its saved annotation
before you run the conversion again. Do not delete the saved state without
restoring its settings.

## Check the result

Check all node InternalIPs against their Flannel public-IP annotations. Check
that each `flannel.1` tunnel uses its node address. Confirm that kube-vip has one
Ready Pod on each control plane and that the VIP is on `foundry-vip`. Run allowed
and denied server admission dry-runs through each control-plane address and
the VIP. A Ready webhook Pod alone does not prove admission connectivity.
