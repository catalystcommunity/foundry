package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Node represents a Kubernetes node with simplified fields
type Node struct {
	Name               string
	Status             string
	Ready              bool
	Roles              []string
	Version            string
	InternalIP         string
	ExternalIP         string
	Conditions         []NodeCondition
	CreationTimestamp  time.Time
	AllocatableCPU     *string
	AllocatableMemory  *string
}

// NodeCondition represents a node condition
type NodeCondition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// Pod represents a Kubernetes pod with simplified fields
type Pod struct {
	Name              string
	Namespace         string
	Status            string
	Phase             corev1.PodPhase
	Ready             bool
	NodeName          string
	PodIP             string
	CreationTimestamp time.Time
	Containers        []Container
}

// Namespace represents a Kubernetes namespace
type Namespace struct {
	Name              string
	Status            string
	CreationTimestamp time.Time
}

// Container represents a container within a pod
type Container struct {
	Name    string
	Image   string
	Ready   bool
	State   string
	Restarts int32
}

// NodeFromCoreV1 converts a core/v1 Node to our Node type
func NodeFromCoreV1(node *corev1.Node) *Node {
	n := &Node{
		Name:              node.Name,
		Version:           node.Status.NodeInfo.KubeletVersion,
		CreationTimestamp: node.CreationTimestamp.Time,
		Roles:             extractNodeRoles(node),
	}

	// Extract IPs
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			n.InternalIP = addr.Address
		case corev1.NodeExternalIP:
			n.ExternalIP = addr.Address
		}
	}

	// Extract allocatable resources
	if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
		cpuStr := cpu.String()
		n.AllocatableCPU = &cpuStr
	}
	if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
		memStr := mem.String()
		n.AllocatableMemory = &memStr
	}

	// Extract conditions
	n.Conditions = make([]NodeCondition, 0, len(node.Status.Conditions))
	for _, cond := range node.Status.Conditions {
		n.Conditions = append(n.Conditions, NodeCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})

		// Set status and ready based on Ready condition
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				n.Status = "Ready"
				n.Ready = true
			} else {
				n.Status = "NotReady"
				n.Ready = false
			}
		}
	}

	return n
}

// extractNodeRoles extracts node roles from labels
func extractNodeRoles(node *corev1.Node) []string {
	roles := []string{}

	// Check for role labels (both old and new formats)
	for label := range node.Labels {
		switch label {
		case "node-role.kubernetes.io/master",
			"node-role.kubernetes.io/control-plane":
			roles = append(roles, "control-plane")
		case "node-role.kubernetes.io/worker":
			roles = append(roles, "worker")
		}
	}

	// If no roles found, default to worker
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}

	return roles
}

// PodFromCoreV1 converts a core/v1 Pod to our Pod type
func PodFromCoreV1(pod *corev1.Pod) *Pod {
	p := &Pod{
		Name:              pod.Name,
		Namespace:         pod.Namespace,
		Phase:             pod.Status.Phase,
		Status:            string(pod.Status.Phase),
		Ready:             isPodReady(pod),
		NodeName:          pod.Spec.NodeName,
		PodIP:             pod.Status.PodIP,
		CreationTimestamp: pod.CreationTimestamp.Time,
		Containers:        make([]Container, 0, len(pod.Status.ContainerStatuses)),
	}

	// Extract container info
	for _, cs := range pod.Status.ContainerStatuses {
		container := Container{
			Name:     cs.Name,
			Image:    cs.Image,
			Ready:    cs.Ready,
			Restarts: cs.RestartCount,
		}

		// Determine container state
		if cs.State.Running != nil {
			container.State = "Running"
		} else if cs.State.Waiting != nil {
			container.State = "Waiting"
		} else if cs.State.Terminated != nil {
			container.State = "Terminated"
		}

		p.Containers = append(p.Containers, container)
	}

	return p
}

// isPodReady checks if all containers in a pod are ready
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
