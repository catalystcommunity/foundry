package kyverno

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	kyaml "sigs.k8s.io/yaml"
)

func testPolicyExpressionRegressions(t *testing.T, ctx context.Context, client dynamic.Interface, ns string,
	apply func(*testing.T, []Selection), selectOnly func(string, State) []Selection,
	admit func(*testing.T, schema.GroupVersionResource, string, *unstructured.Unstructured, string)) {
	pods := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	makePod := func(spec string) *unstructured.Unstructured {
		t.Helper()
		var fields map[string]interface{}
		require.NoError(t, kyaml.Unmarshal([]byte(spec), &fields))
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Pod", "metadata": map[string]interface{}{"name": "expression-check"}, "spec": fields,
		}}
	}
	checkPod := func(t *testing.T, name, spec string, allowed bool) {
		t.Helper()
		denied := name
		if allowed {
			denied = ""
		}
		admit(t, pods, ns, makePod(spec), denied)
	}
	// An ephemeral container enters through an UPDATE subresource, not Pod CREATE.
	checkEphemeral := func(t *testing.T, policy, baseSpec, containerYAML string, allowed bool) {
		t.Helper()
		base := makePod(baseSpec)
		base.SetName("")
		base.SetGenerateName("ephemeral-check-")
		require.NoError(t, unstructured.SetNestedField(base.Object, "foundry-test-unscheduled", "spec", "nodeName"))
		resource := client.Resource(pods).Namespace(ns)
		created, err := resource.Create(ctx, base, metav1.CreateOptions{})
		require.NoError(t, err)
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			zero := int64(0)
			require.NoError(t, resource.Delete(cleanup, created.GetName(), metav1.DeleteOptions{GracePeriodSeconds: &zero}))
		})
		var container map[string]interface{}
		require.NoError(t, kyaml.Unmarshal([]byte(containerYAML), &container))
		require.NoError(t, unstructured.SetNestedSlice(created.Object, []interface{}{container}, "spec", "ephemeralContainers"))
		var lastErr error
		converged := assert.Eventually(t, func() bool {
			_, lastErr = resource.Update(ctx, created, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}, "ephemeralcontainers")
			if allowed {
				return lastErr == nil
			}
			return lastErr != nil && strings.Contains(lastErr.Error(), policy)
		}, 30*time.Second, 500*time.Millisecond)
		if !converged {
			t.Fatalf("ephemeral admission error: %v", lastErr)
		}
		if !allowed {
			require.NotContains(t, lastErr.Error(), "no such key")
			require.NotContains(t, lastErr.Error(), "no such overload")
		}
	}

	t.Run("strict seccomp", func(t *testing.T) {
		name := "restrict-seccomp-strict"
		apply(t, selectOnly(name, StateEnforce))
		for _, tc := range []struct {
			name, spec string
			allowed    bool
		}{
			{"missing", "containers: [{name: app, image: nginx:1.27}]", false},
			{"inherited runtime", "securityContext: {seccompProfile: {type: RuntimeDefault}}\ncontainers: [{name: app, image: nginx:1.27}]", true},
			{"inherited localhost", "securityContext: {seccompProfile: {type: Localhost, localhostProfile: profiles/test.json}}\ncontainers: [{name: app, image: nginx:1.27}]", true},
			{"container runtime", "containers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: RuntimeDefault}}}]", true},
			{"container localhost", "containers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: Localhost, localhostProfile: profiles/test.json}}}]", true},
			{"container unconfined", "securityContext: {seccompProfile: {type: RuntimeDefault}}\ncontainers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: Unconfined}}}]", false},
			{"pod unconfined", "securityContext: {seccompProfile: {type: Unconfined}}\ncontainers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: RuntimeDefault}}}]", false},
			{"one container missing", "containers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: RuntimeDefault}}}, {name: second, image: nginx:1.27}]", false},
			{"init missing", "containers: [{name: app, image: nginx:1.27, securityContext: {seccompProfile: {type: RuntimeDefault}}}]\ninitContainers: [{name: init, image: busybox:1.36}]", false},
			{"init inherited", "securityContext: {seccompProfile: {type: RuntimeDefault}}\ncontainers: [{name: app, image: nginx:1.27}]\ninitContainers: [{name: init, image: busybox:1.36}]", true},
		} {
			t.Run(tc.name, func(t *testing.T) { checkPod(t, name, tc.spec, tc.allowed) })
		}
		for _, tc := range []struct {
			name, security string
			allowed        bool
		}{
			{"inherited", "", true}, {"unconfined", ", securityContext: {seccompProfile: {type: Unconfined}}", false},
		} {
			t.Run("ephemeral "+tc.name, func(t *testing.T) {
				checkEphemeral(t, name, "securityContext: {seccompProfile: {type: RuntimeDefault}}\ncontainers: [{name: app, image: nginx:1.27}]",
					"{name: debug, image: busybox:1.36"+tc.security+"}", tc.allowed)
			})
		}
	})

	t.Run("drop NET_RAW", func(t *testing.T) {
		name := "drop-cap-net-raw"
		apply(t, selectOnly(name, StateEnforce))
		for _, tc := range []struct {
			name, capabilities string
			allowed            bool
		}{
			{"drop all", "{drop: [ALL]}", true},
			{"drop net raw", "{drop: [NET_RAW]}", true},
			{"prefixed name", "{drop: [CAP_NET_RAW]}", true},
			{"lower case", "{drop: [all]}", true},
			{"empty add", "{drop: [ALL], add: []}", true},
			{"other capability", "{drop: [ALL], add: [NET_BIND_SERVICE]}", true},
			{"add net raw back", "{drop: [ALL], add: [NET_RAW]}", false},
			{"add prefixed name back", "{drop: [NET_RAW], add: [CAP_NET_RAW]}", false},
			{"add lower case back", "{drop: [ALL], add: [net_raw]}", false},
			{"add all back", "{drop: [NET_RAW], add: [ALL]}", false},
			{"unrelated drop", "{drop: [CHOWN]}", false},
			{"missing drop", "{add: [NET_BIND_SERVICE]}", false},
			{"empty drop", "{drop: []}", false},
			{"empty capabilities", "{}", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				checkPod(t, name, "containers: [{name: app, image: nginx:1.27, securityContext: {capabilities: "+tc.capabilities+"}}]", tc.allowed)
			})
		}
		valid := "containers: [{name: app, image: nginx:1.27, securityContext: {capabilities: {drop: [ALL]}}}]"
		checkPod(t, name, "containers: [{name: app, image: nginx:1.27}]", false)
		checkPod(t, name, valid+"\ninitContainers: [{name: init, image: busybox:1.36}]", false)
		checkPod(t, name, valid+"\ninitContainers: [{name: init, image: busybox:1.36, securityContext: {capabilities: {drop: [ALL]}}}]", true)
		checkPod(t, name, valid+"\ninitContainers: [{name: init, image: busybox:1.36, securityContext: {capabilities: {drop: [ALL], add: [NET_RAW]}}}]", false)
		for _, allowed := range []bool{true, false} {
			spec := valid
			denied := ""
			if !allowed {
				spec = "containers: [{name: app, image: nginx:1.27}]"
				denied = name
			}
			deployment := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "capabilities-check"},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "capabilities-check"}},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{"labels": map[string]interface{}{"app": "capabilities-check"}},
						"spec":     makePod(spec).Object["spec"],
					},
				},
			}}
			admit(t, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, ns, deployment, denied)
		}
		apply(t, selectOnly(name, StateDisabled))
		checkPod(t, name, "containers: [{name: app, image: nginx:1.27}]", true)
	})

	t.Run("strict capabilities", func(t *testing.T) {
		name := "disallow-capabilities-strict"
		apply(t, selectOnly(name, StateEnforce))
		for _, tc := range []struct {
			name, capabilities string
			allowed            bool
		}{
			{"drop all", "{drop: [ALL]}", true},
			{"empty add", "{drop: [ALL], add: []}", true},
			{"permitted add", "{drop: [ALL], add: [NET_BIND_SERVICE]}", true},
			{"forbidden add", "{drop: [ALL], add: [CHOWN]}", false},
			{"mixed add", "{drop: [ALL], add: [NET_BIND_SERVICE, CHOWN]}", false},
			{"missing drop", "{add: [NET_BIND_SERVICE]}", false},
			{"empty capabilities", "{}", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				checkPod(t, name, "containers: [{name: app, image: nginx:1.27, securityContext: {capabilities: "+tc.capabilities+"}}]", tc.allowed)
			})
		}
		checkPod(t, name, "containers: [{name: app, image: nginx:1.27}]", false)
	})

	for _, name := range []string{"restrict-wildcard-resources", "restrict-wildcard-verbs", "restrict-secret-role-verbs", "restrict-escalation-verbs-roles"} {
		t.Run(name, func(t *testing.T) {
			apply(t, selectOnly(name, StateEnforce))
			cases := []struct {
				name, kind, rules string
				allowed           bool
			}{
				{"empty role", "Role", "", true}, {"empty cluster role", "ClusterRole", "", true},
				{"empty list", "ClusterRole", "[]", true}, {"null rules", "ClusterRole", "null", true},
				{"nonresource", "ClusterRole", "[{nonResourceURLs: ['/healthz'], verbs: [get]}]", true},
				{"read pods", "Role", "[{apiGroups: [''], resources: [pods], verbs: [get]}]", true},
				{"wildcard resources", "ClusterRole", "[{apiGroups: [''], resources: ['*'], verbs: [get]}]", name != "restrict-wildcard-resources"},
				{"wildcard verbs", "ClusterRole", "[{apiGroups: [''], resources: [pods], verbs: ['*']}]", name != "restrict-wildcard-verbs"},
				{"read secrets", "Role", "[{apiGroups: [''], resources: [secrets], verbs: [get]}]", name != "restrict-secret-role-verbs"},
				{"write secrets", "Role", "[{apiGroups: [''], resources: [secrets], verbs: [create]}]", true},
				{"bind", "ClusterRole", "[{apiGroups: [rbac.authorization.k8s.io], resources: [roles], verbs: [bind]}]", name != "restrict-escalation-verbs-roles"},
				{"escalate", "ClusterRole", "[{apiGroups: [rbac.authorization.k8s.io], resources: [clusterroles], verbs: [escalate]}]", name != "restrict-escalation-verbs-roles"},
			}
			if name == "restrict-escalation-verbs-roles" {
				for _, target := range []struct{ group, resource string }{
					{"", "users"}, {"", "groups"}, {"", "serviceaccounts"}, {"authentication.k8s.io", "uids"},
					{"authentication.k8s.io", "userextras/scopes"}, {"authentication.k8s.io", "userextras/*"}, {"*", "*"},
				} {
					cases = append(cases, struct {
						name, kind, rules string
						allowed           bool
					}{
						"impersonate " + target.group + " " + target.resource, "ClusterRole",
						fmt.Sprintf("[{apiGroups: [%q], resources: [%q], verbs: [impersonate]}]", target.group, target.resource), false,
					})
				}
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					object := &unstructured.Unstructured{Object: map[string]interface{}{
						"apiVersion": "rbac.authorization.k8s.io/v1", "kind": tc.kind, "metadata": map[string]interface{}{"name": "rbac-check"},
					}}
					if tc.rules != "" {
						var fields map[string]interface{}
						require.NoError(t, kyaml.Unmarshal([]byte("rules: "+tc.rules), &fields))
						object.Object["rules"] = fields["rules"]
					}
					resource, namespace := "roles", ns
					if tc.kind == "ClusterRole" {
						resource, namespace = "clusterroles", ""
					}
					denied := name
					if tc.allowed {
						denied = ""
					}
					admit(t, schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: resource}, namespace, object, denied)
				})
			}
		})
	}

	digest := "nginx@sha256:" + strings.Repeat("a", 64)
	for _, name := range []string{"disallow-latest-tag", "require-image-checksum"} {
		t.Run(name, func(t *testing.T) {
			apply(t, selectOnly(name, StateEnforce))
			for _, tc := range []struct {
				name, image string
				tagged      bool
			}{
				{"untagged", "nginx", false}, {"latest", "nginx:latest", false},
				{"registry port untagged", "registry.example:5000/team/app", false},
				{"registry port latest", "registry.example:5000/team/app:latest", false},
				{"registry port version", "registry.example:5000/team/app:1.2", true},
				{"version", "nginx:1.27", true}, {"digest", digest, true},
				{"latest with digest", "nginx:latest@sha256:" + strings.Repeat("a", 64), true},
			} {
				for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
					t.Run(tc.name+" "+field, func(t *testing.T) {
						allowed := tc.tagged
						if name == "require-image-checksum" {
							allowed = strings.Contains(tc.image, "@")
						}
						container := fmt.Sprintf("{name: check, image: %q}", tc.image)
						base := fmt.Sprintf("containers: [{name: app, image: %q}]", digest)
						if field == "ephemeralContainers" {
							checkEphemeral(t, name, base, container, allowed)
						} else if field == "containers" {
							checkPod(t, name, "containers: ["+container+"]", allowed)
						} else {
							checkPod(t, name, base+"\ninitContainers: ["+container+"]", allowed)
						}
					})
				}
			}
		})
	}
}
