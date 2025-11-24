#!/bin/bash
#
# Migration script: Simple pattern replacements for role-based hosts
# This handles VIP renaming and cluster.Nodes → role-based discovery
#

set -e

REPO_ROOT="/home/todhansmann/repos/catalystcommunity/foundry/v1"
cd "$REPO_ROOT"

echo "=== Simple Pattern Migration ==="
echo ""

# Backup first
BACKUP_DIR="/tmp/foundry-migration-backup-$(date +%Y%m%d-%H%M%S)"
echo "Creating backup at: $BACKUP_DIR"
mkdir -p "$BACKUP_DIR"
cp -r cmd/foundry/commands "$BACKUP_DIR/"
cp -r internal "$BACKUP_DIR/"

echo "Backup complete"
echo ""

echo "=== Phase 1: VIP Renaming ==="
echo "  Network.K8sVIP → Cluster.VIP"
echo ""

# Find all .go files and replace Network.K8sVIP with Cluster.VIP
find cmd/foundry/commands -name "*.go" -type f ! -name "*_test.go" | while read file; do
    if grep -q "Network\.K8sVIP" "$file" 2>/dev/null; then
        echo "  Updating: $file"
        sed -i 's/Network\.K8sVIP/Cluster.VIP/g' "$file"
    fi
done

find internal -name "*.go" -type f ! -name "*_test.go" | while read file; do
    if grep -q "Network\.K8sVIP" "$file" 2>/dev/null; then
        echo "  Updating: $file"
        sed -i 's/Network\.K8sVIP/Cluster.VIP/g' "$file"
    fi
done

echo ""
echo "=== Phase 2: Display Updates ==="
echo "  Updating display strings (non-critical)"
echo ""

# Update display strings that are safe to change
find cmd/foundry/commands -name "*.go" -type f ! -name "*_test.go" | while read file; do
    if grep -q 'network\.k8s_vip' "$file" 2>/dev/null; then
        echo "  Updating display string in: $file"
        sed -i 's/network\.k8s_vip/cluster.vip/g' "$file"
    fi
done

echo ""
echo "=== Migration Complete ==="
echo ""
echo "Changes made:"
echo "  ✓ Network.K8sVIP → Cluster.VIP (all non-test files)"
echo "  ✓ Display strings updated"
echo ""
echo "Backup location: $BACKUP_DIR"
echo ""
echo "Next: Manual updates needed for complex patterns in:"
echo "  - cmd/foundry/commands/stack/install.go (host array references)"
echo "  - cmd/foundry/commands/cluster/*.go (Cluster.Nodes references)"
echo "  - cmd/foundry/commands/network/*.go (host validation)"
echo ""
