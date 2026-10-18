set -eu
PATH=/usr/sbin:/usr/bin:/sbin:/bin
key=$1
minimum=$2
config=$3

is_positive_integer() {
    case "$1" in ''|*[!0-9]*) return 1 ;; esac
    [ "$1" -gt 0 ]
}

current=$(sysctl -n "$key")
if ! is_positive_integer "$current"; then
    echo "Invalid current value for $key" >&2
    exit 1
fi
target=$minimum
if [ "$current" -gt "$target" ]; then
    target=$current
fi

# Preserve a higher value in the file that Foundry owns, even after a reset.
if [ -e "$config" ]; then
    saved=$(awk -v key="$key" '
        /^[[:space:]]*($|#|;)/ { next }
        { sub(/[[:space:]]*#.*/, ""); gsub(/[[:space:]]/, "") }
        { fields=split($0, parts, "=") }
        fields == 2 && parts[1] == key && parts[2] ~ /^[0-9]+$/ { value=parts[2]; count++; next }
        { invalid=1 }
        END { if (invalid || count != 1) exit 1; print value }
    ' "$config") || { echo "Invalid managed sysctl file: $config" >&2; exit 1; }
    if ! is_positive_integer "$saved"; then
        echo "Invalid saved value for $key" >&2
        exit 1
    fi
    if [ "$saved" -gt "$target" ]; then
        target=$saved
    fi
fi

mkdir -p "$(dirname "$config")"
temporary=$(mktemp "${config}.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
printf '# Managed by Foundry. Preserve higher limits.\n%s = %s\n' "$key" "$target" > "$temporary"
chmod 0644 "$temporary"
if ! cmp -s "$temporary" "$config"; then
    mv -f "$temporary" "$config"
fi

if [ "$current" -lt "$target" ]; then
    sysctl -p "$config"
fi
actual=$(sysctl -n "$key")
if ! is_positive_integer "$actual" || [ "$actual" -lt "$target" ]; then
    echo "Failed to verify $key minimum $target" >&2
    exit 1
fi
