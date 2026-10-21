#!/usr/bin/env python3
# Managed by Foundry. Keep floating addresses off the Flannel interface.
import json
import subprocess
import sys

NAME = 'foundry-vip'
OWNER = 'foundry:vip-interface:v1'

def run(*args):
    return subprocess.check_output(args, text=True).strip()

def ensure(parent):
    links = json.loads(run('ip', '-j', '-d', 'link', 'show'))
    base = next((x for x in links if x['ifname'] == parent), None)
    if not base or base.get('link_type') != 'ether' or 'NOARP' in base.get('flags', []):
        raise RuntimeError('VIP parent must be an Ethernet interface with ARP; do not use tailscale0')
    if 'UP' not in base.get('flags', []):
        raise RuntimeError('VIP parent interface is not up')
    child = next((x for x in links if x['ifname'] == NAME), None)
    if child:
        info = child.get('linkinfo', {})
        if (child.get('ifalias') != OWNER or info.get('info_kind') != 'macvlan'
                or info.get('info_data', {}).get('mode') != 'bridge'
                or not (child.get('link_index') == base['ifindex'] or child.get('link') == parent)):
            raise RuntimeError('Refusing to replace an incompatible or unmanaged foundry-vip interface')
    else:
        run('ip', 'link', 'add', 'link', parent, 'name', NAME,
            'type', 'macvlan', 'mode', 'bridge')
        run('ip', 'link', 'set', 'dev', NAME, 'alias', OWNER)
    run('ip', 'link', 'set', 'dev', NAME, 'mtu', str(base['mtu']), 'up')
    # Replies can use the primary NIC. Strict reverse-path filtering would
    # discard requests arriving on the VIP NIC from the same LAN.
    run('sysctl', '-q', '-w', 'net.ipv4.conf.' + NAME + '.rp_filter=2')

if __name__ == '__main__':
    try:
        ensure(sys.argv[1])
    except Exception as error:
        sys.exit('Foundry VIP interface: ' + str(error))
