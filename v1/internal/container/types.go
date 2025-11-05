package container

import (
	"time"
)

// Runtime represents a container runtime (Docker, Podman, etc.)
type Runtime interface {
	// Name returns the name of the runtime (e.g., "docker", "podman")
	Name() string

	// Pull pulls a container image
	Pull(image string) error

	// Run runs a container with the given configuration
	// Returns the container ID
	Run(config RunConfig) (string, error)

	// Stop stops a running container
	Stop(containerID string, timeout time.Duration) error

	// Remove removes a container
	Remove(containerID string, force bool) error

	// Inspect inspects a container and returns its details
	Inspect(containerID string) (*ContainerInfo, error)

	// List lists all containers (running and stopped)
	List(all bool) ([]ContainerInfo, error)

	// IsAvailable checks if the runtime is available on the system
	IsAvailable() bool
}

// RunConfig contains configuration for running a container
type RunConfig struct {
	// Image is the container image to run
	Image string

	// Name is the optional container name
	Name string

	// Detach runs the container in the background
	Detach bool

	// Remove removes the container when it exits (--rm)
	Remove bool

	// Ports maps host ports to container ports (e.g., "8080:80")
	Ports []string

	// Volumes maps host paths to container paths (e.g., "/host/path:/container/path")
	Volumes []string

	// Env sets environment variables (e.g., "KEY=value")
	Env []string

	// Command is the command to run in the container
	Command []string

	// Network specifies the network to connect to
	Network string

	// RestartPolicy sets the restart policy (e.g., "always", "unless-stopped")
	RestartPolicy string

	// User specifies the user to run as
	User string

	// WorkDir sets the working directory
	WorkDir string

	// Labels sets container labels
	Labels map[string]string

	// Privileged runs the container in privileged mode
	Privileged bool
}

// ContainerInfo represents information about a container
type ContainerInfo struct {
	// ID is the container ID
	ID string

	// Name is the container name
	Name string

	// Image is the image used by the container
	Image string

	// State is the container state (running, exited, etc.)
	State string

	// Status provides additional status information
	Status string

	// Created is when the container was created
	Created time.Time

	// Ports lists the port mappings
	Ports []string

	// Labels contains the container labels
	Labels map[string]string
}

// ImageInfo represents information about a container image
type ImageInfo struct {
	// ID is the image ID
	ID string

	// RepoTags are the repository tags (e.g., "nginx:latest")
	RepoTags []string

	// Created is when the image was created
	Created time.Time

	// Size is the image size in bytes
	Size int64
}
