// Package gatewayroute builds HTTPRoute resources for Foundry components.
package gatewayroute

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	gatewayName      = "contour"
	gatewayNamespace = "projectcontour"
)

// Ownership describes the manager metadata for a route.
type Ownership struct {
	Component        string
	ManagedBy        string
	ReleaseName      string
	ReleaseNamespace string
}

// BackendOptions describes a route from one or more Gateway listeners to a
// Service in the route namespace.
type BackendOptions struct {
	Name           string
	Namespace      string
	Hostname       string
	ParentSections []string
	ServiceName    string
	ServicePort    int
	Ownership      Ownership
}

// RedirectOptions describes an HTTP route that redirects clients to HTTPS.
type RedirectOptions struct {
	Name      string
	Namespace string
	Hostname  string
	Ownership Ownership
}

// Backend builds an HTTPRoute that sends traffic to a Service.
func Backend(opts BackendOptions) map[string]interface{} {
	parentRefs := make([]interface{}, 0, len(opts.ParentSections))
	for _, section := range opts.ParentSections {
		parentRefs = append(parentRefs, map[string]interface{}{
			"name":        gatewayName,
			"namespace":   gatewayNamespace,
			"sectionName": section,
		})
	}

	return map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   metadata(opts.Name, opts.Namespace, opts.Ownership),
		"spec": map[string]interface{}{
			"parentRefs": parentRefs,
			"hostnames":  []interface{}{opts.Hostname},
			"rules": []interface{}{
				map[string]interface{}{
					"backendRefs": []interface{}{
						map[string]interface{}{
							"name": opts.ServiceName,
							"port": opts.ServicePort,
						},
					},
				},
			},
		},
	}
}

// RedirectToHTTPS builds an HTTPRoute that redirects plaintext requests to
// the same host and path over HTTPS.
func RedirectToHTTPS(opts RedirectOptions) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   metadata(opts.Name, opts.Namespace, opts.Ownership),
		"spec": map[string]interface{}{
			"parentRefs": []interface{}{
				map[string]interface{}{
					"name":        gatewayName,
					"namespace":   gatewayNamespace,
					"sectionName": "http",
				},
			},
			"hostnames": []interface{}{opts.Hostname},
			"rules": []interface{}{
				map[string]interface{}{
					"filters": []interface{}{
						map[string]interface{}{
							"type": "RequestRedirect",
							"requestRedirect": map[string]interface{}{
								"scheme":     "https",
								"statusCode": 301,
							},
						},
					},
				},
			},
		},
	}
}

// AppendObjects preserves user objects and appends Foundry-managed objects to
// a chart value such as extraObjects or extraManifests.
func AppendObjects(values map[string]interface{}, key string, objects ...map[string]interface{}) {
	items := make([]interface{}, 0, len(objects))
	if existing, ok := values[key]; ok {
		switch typed := existing.(type) {
		case []interface{}:
			items = append(items, typed...)
		case []map[string]interface{}:
			for _, item := range typed {
				items = append(items, item)
			}
		case map[string]interface{}:
			items = append(items, typed)
		}
	}
	for _, object := range objects {
		items = append(items, object)
	}
	values[key] = items
}

// Manifest serializes resources as a multi-document manifest.
func Manifest(objects ...map[string]interface{}) (string, error) {
	documents := make([]string, 0, len(objects))
	for _, object := range objects {
		data, err := json.Marshal(object)
		if err != nil {
			return "", fmt.Errorf("marshal Gateway API resource: %w", err)
		}
		documents = append(documents, string(data))
	}
	return strings.Join(documents, "\n---\n"), nil
}

func metadata(name, namespace string, ownership Ownership) map[string]interface{} {
	managedBy := ownership.ManagedBy
	if managedBy == "" {
		managedBy = "foundry"
	}
	labels := map[string]interface{}{
		"app.kubernetes.io/managed-by": managedBy,
	}
	if ownership.Component != "" {
		labels["app.kubernetes.io/component"] = ownership.Component
	}

	result := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
		"labels":    labels,
	}
	if ownership.ReleaseName != "" && ownership.ReleaseNamespace != "" {
		result["annotations"] = map[string]interface{}{
			"meta.helm.sh/release-name":      ownership.ReleaseName,
			"meta.helm.sh/release-namespace": ownership.ReleaseNamespace,
		}
	}
	return result
}
