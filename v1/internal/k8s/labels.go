package k8s

import "strings"

// System label prefixes that should not be managed by users
var systemLabelPrefixes = []string{
	"kubernetes.io/",
	"k8s.io/",
	"node-role.kubernetes.io/",
	"node.kubernetes.io/",
}

// IsSystemLabel returns true if the label key is a system-managed label
// that should not be modified by users
func IsSystemLabel(key string) bool {
	for _, prefix := range systemLabelPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// FilterUserLabels returns only user-manageable labels (excludes system labels)
func FilterUserLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		if !IsSystemLabel(k) {
			result[k] = v
		}
	}
	return result
}

// FilterSystemLabels returns only system-managed labels
func FilterSystemLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		if IsSystemLabel(k) {
			result[k] = v
		}
	}
	return result
}
