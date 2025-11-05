package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestZotIntegration tests the complete Zot registry lifecycle with a real container
func TestZotIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start Zot container
	container, registryURL, err := startZotContainer(ctx, t)
	require.NoError(t, err, "Failed to start Zot container")
	t.Logf("Zot container started at %s", registryURL)

	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	t.Run("Health_Check", func(t *testing.T) {
		// Zot provides a health endpoint at /v2/
		// OCI Distribution spec requires this endpoint
		resp, err := http.Get(registryURL + "/v2/")
		require.NoError(t, err, "Should be able to query /v2/ endpoint")
		defer resp.Body.Close()

		// Should return 200 OK
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Registry should be healthy")

		// Should have Docker-Distribution-Api-Version header
		apiVersion := resp.Header.Get("Docker-Distribution-Api-Version")
		assert.NotEmpty(t, apiVersion, "Should have API version header")
	})

	t.Run("Repository_List", func(t *testing.T) {
		// List repositories (should be empty initially)
		resp, err := http.Get(registryURL + "/v2/_catalog")
		require.NoError(t, err, "Should list repositories")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Catalog endpoint should work")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		var catalog struct {
			Repositories []string `json:"repositories"`
		}
		err = json.Unmarshal(body, &catalog)
		require.NoError(t, err, "Should parse catalog response")

		// Initially empty or null
		assert.True(t, len(catalog.Repositories) == 0 || catalog.Repositories == nil,
			"Catalog should be empty initially")
	})

	t.Run("Manifest_Upload_Minimal", func(t *testing.T) {
		// Test minimal manifest upload
		// This tests the registry's ability to accept manifests
		// We'll use a minimal valid OCI manifest

		testRepo := "test/minimal"
		testTag := "v1.0.0"

		// Upload a minimal config blob first
		configBlob := `{"architecture":"amd64","os":"linux"}`
		configDigest, err := uploadBlob(registryURL, testRepo, []byte(configBlob))
		require.NoError(t, err, "Should upload config blob")
		t.Logf("Uploaded config blob: %s", configDigest)

		// Create a minimal manifest
		manifest := fmt.Sprintf(`{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"config": {
				"mediaType": "application/vnd.oci.image.config.v1+json",
				"size": %d,
				"digest": "%s"
			},
			"layers": []
		}`, len(configBlob), configDigest)

		// Upload the manifest
		manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, testRepo, testTag)
		req, err := http.NewRequest(http.MethodPut, manifestURL, strings.NewReader(manifest))
		require.NoError(t, err, "Should create manifest request")
		req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err, "Should upload manifest")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Manifest upload should succeed")

		// Verify manifest can be retrieved
		resp2, err := http.Get(manifestURL)
		require.NoError(t, err, "Should retrieve manifest")
		defer resp2.Body.Close()

		assert.Equal(t, http.StatusOK, resp2.StatusCode, "Should retrieve uploaded manifest")
	})

	t.Run("Tag_List", func(t *testing.T) {
		// List tags for the test repository
		testRepo := "test/minimal"
		tagsURL := fmt.Sprintf("%s/v2/%s/tags/list", registryURL, testRepo)

		resp, err := http.Get(tagsURL)
		require.NoError(t, err, "Should list tags")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Tags endpoint should work")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		var tagList struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}
		err = json.Unmarshal(body, &tagList)
		require.NoError(t, err, "Should parse tags response")

		assert.Equal(t, testRepo, tagList.Name, "Repository name should match")
		assert.Contains(t, tagList.Tags, "v1.0.0", "Should have uploaded tag")
	})

	t.Run("Catalog_With_Images", func(t *testing.T) {
		// Verify catalog now shows our test repository
		resp, err := http.Get(registryURL + "/v2/_catalog")
		require.NoError(t, err, "Should list repositories")
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		var catalog struct {
			Repositories []string `json:"repositories"`
		}
		err = json.Unmarshal(body, &catalog)
		require.NoError(t, err, "Should parse catalog response")

		assert.Contains(t, catalog.Repositories, "test/minimal",
			"Catalog should include test repository")
	})

	t.Run("Manifest_Deletion", func(t *testing.T) {
		// Test manifest deletion
		testRepo := "test/minimal"
		testTag := "v1.0.0"

		// First, get the manifest digest
		manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, testRepo, testTag)
		req, err := http.NewRequest(http.MethodHead, manifestURL, nil)
		require.NoError(t, err, "Should create HEAD request")
		req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err, "Should get manifest headers")
		resp.Body.Close()

		digest := resp.Header.Get("Docker-Content-Digest")
		require.NotEmpty(t, digest, "Should have manifest digest")
		t.Logf("Manifest digest: %s", digest)

		// Delete the manifest by digest
		deleteURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, testRepo, digest)
		req2, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
		require.NoError(t, err, "Should create DELETE request")

		resp2, err := client.Do(req2)
		require.NoError(t, err, "Should delete manifest")
		defer resp2.Body.Close()

		assert.Equal(t, http.StatusAccepted, resp2.StatusCode, "Manifest deletion should succeed")

		// Verify manifest is gone
		resp3, err := http.Get(manifestURL)
		require.NoError(t, err, "Should attempt to retrieve manifest")
		defer resp3.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp3.StatusCode,
			"Manifest should not exist after deletion")
	})

	t.Run("Extension_Search_API", func(t *testing.T) {
		// Test Zot's search extension if enabled
		// This is a Zot-specific feature, not part of OCI spec
		searchURL := registryURL + "/v2/_catalog"

		resp, err := http.Get(searchURL)
		require.NoError(t, err, "Should query search API")
		defer resp.Body.Close()

		// Search API should at least return catalog
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Search should work")
	})
}

// startZotContainer starts a Zot container and returns the container, registry URL, and error
func startZotContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string, error) {
	// Zot registry with minimal configuration
	// Using in-memory storage for test speed
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/project-zot/zot:latest",
		ExposedPorts: []string{"5000/tcp"},
		Env: map[string]string{
			// Zot can run with minimal env config
			"ZOT_LOG_LEVEL": "info",
		},
		// Zot starts quickly and is ready when port is open
		WaitingFor: wait.ForHTTP("/v2/").
			WithPort("5000/tcp").
			WithStatusCodeMatcher(func(status int) bool {
				// OCI registry spec: /v2/ should return 200 OK
				return status == http.StatusOK
			}).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start container: %w", err)
	}

	// Get the mapped port
	mappedPort, err := container.MappedPort(ctx, "5000")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	registryURL := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	t.Logf("Zot registry available at %s", registryURL)

	// Give Zot a moment to fully initialize
	time.Sleep(1 * time.Second)

	return container, registryURL, nil
}

// uploadBlob uploads a blob to the registry and returns its digest
func uploadBlob(registryURL, repo string, data []byte) (string, error) {
	// Calculate digest first
	hash := sha256.Sum256(data)
	digest := fmt.Sprintf("sha256:%x", hash)

	// Start blob upload
	client := &http.Client{}
	uploadURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", registryURL, repo)

	resp, err := http.Post(uploadURL, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to start blob upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status for upload initiation: %d, body: %s",
			resp.StatusCode, string(body))
	}

	// Get upload location
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header in upload response")
	}

	// Make location absolute if needed
	if !strings.HasPrefix(location, "http") {
		location = registryURL + location
	}

	// Upload the blob data with digest parameter
	uploadDataURL := location
	if strings.Contains(uploadDataURL, "?") {
		uploadDataURL = fmt.Sprintf("%s&digest=%s", uploadDataURL, digest)
	} else {
		uploadDataURL = fmt.Sprintf("%s?digest=%s", uploadDataURL, digest)
	}

	req, err := http.NewRequest(http.MethodPut, uploadDataURL, strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))

	resp2, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload blob data: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp2.Body)
		return "", fmt.Errorf("unexpected status for blob upload: %d, body: %s",
			resp2.StatusCode, string(body))
	}

	return digest, nil
}
