package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestOpenBAOIntegration tests the complete OpenBAO lifecycle with a real container
func TestOpenBAOIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start OpenBAO container and get root token from logs
	container, apiURL, rootToken, err := startOpenBAOContainer(ctx, t)
	require.NoError(t, err, "Failed to start OpenBAO container")
	require.NotEmpty(t, rootToken, "Root token should be extracted from logs")
	t.Logf("Using root token: %s", rootToken)

	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	t.Run("Health_Check_Ready", func(t *testing.T) {
		client := openbao.NewClient(apiURL, "")

		health, err := client.Health(ctx)
		require.NoError(t, err, "Health check should succeed")
		assert.True(t, health.Initialized, "Should be initialized (dev mode)")
		assert.False(t, health.Sealed, "Should be unsealed (dev mode)")
		assert.Equal(t, 200, health.StatusCode, "Status code should be 200 (ready)")
		assert.True(t, health.IsReady(), "Should be ready")
	})

	t.Run("Secret_Storage_And_Retrieval", func(t *testing.T) {
		client := openbao.NewClient(apiURL, rootToken)

		// Dev mode has KV v2 enabled at 'secret' mount by default
		testPath := "test/my-secret"
		testData := map[string]interface{}{
			"username": "admin",
			"password": "super-secret-password",
			"api_key":  "abc123def456",
		}

		// Write secret using KV v2 API
		err := client.WriteSecretV2(ctx, "secret", testPath, testData)
		require.NoError(t, err, "Should store secret successfully")

		// Retrieve the secret
		secret, err := client.ReadSecretV2(ctx, "secret", testPath)
		require.NoError(t, err, "Should retrieve secret successfully")
		require.NotNil(t, secret, "Secret should not be nil")
		assert.Equal(t, testData["username"], secret["username"], "Username should match")
		assert.Equal(t, testData["password"], secret["password"], "Password should match")
		assert.Equal(t, testData["api_key"], secret["api_key"], "API key should match")
	})

	t.Run("Secret_Resolver", func(t *testing.T) {
		// Create resolver using addr and token
		resolver, err := secrets.NewOpenBAOResolverWithMount(apiURL, rootToken, "secret")
		require.NoError(t, err, "Should create resolver")

		// Store a test secret for resolver
		client := openbao.NewClient(apiURL, rootToken)
		testPath := "database/prod"
		testData := map[string]interface{}{
			"password": "db-password-123",
			"username": "dbuser",
		}
		err = client.WriteSecretV2(ctx, "secret", testPath, testData)
		require.NoError(t, err, "Should store secret")

		// Resolve secret using resolver
		resolutionCtx := &secrets.ResolutionContext{
			Instance: "",
		}
		value, err := resolver.Resolve(resolutionCtx, secrets.SecretRef{
			Path: testPath,
			Key:  "password",
		})
		require.NoError(t, err, "Should resolve secret")
		assert.Equal(t, "db-password-123", value, "Resolved value should match")
	})

	t.Run("SSH_Key_Storage", func(t *testing.T) {
		// For testing, we can use the 'secret' mount that's already available in dev mode
		// In production, this would use the 'foundry-core' mount
		client := openbao.NewClient(apiURL, rootToken)

		// Generate test key pair
		testHost := "test-host.example.com"
		privateKey := []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...(test key)...\n-----END RSA PRIVATE KEY-----")
		publicKey := []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... test@example.com")

		// Store SSH keys directly using KV v2 API at secret/ssh-keys/<host>
		keyData := map[string]interface{}{
			"private": string(privateKey),
			"public":  string(publicKey),
		}
		err := client.WriteSecretV2(ctx, "secret", fmt.Sprintf("ssh-keys/%s", testHost), keyData)
		require.NoError(t, err, "Should store SSH keys")

		// Retrieve SSH keys
		retrieved, err := client.ReadSecretV2(ctx, "secret", fmt.Sprintf("ssh-keys/%s", testHost))
		require.NoError(t, err, "Should retrieve SSH keys")
		require.NotNil(t, retrieved, "Retrieved keys should not be nil")
		assert.Equal(t, string(privateKey), retrieved["private"], "Private key should match")
		assert.Equal(t, string(publicKey), retrieved["public"], "Public key should match")
	})

	t.Run("Auth_Token_Management", func(t *testing.T) {
		// Note: This test uses the actual OS keyring or file fallback
		// We'll test the basic functionality without polluting the user's keyring

		testToken := "test-token-123456"

		// Store token
		err := secrets.StoreAuthToken(testToken)
		require.NoError(t, err, "Should store auth token")

		// Load token
		loaded, err := secrets.LoadAuthToken()
		require.NoError(t, err, "Should load auth token")
		assert.Equal(t, testToken, loaded, "Loaded token should match")

		// Clear token
		err = secrets.ClearAuthToken()
		require.NoError(t, err, "Should clear auth token")

		// Verify cleared
		_, err = secrets.LoadAuthToken()
		assert.Error(t, err, "Should error when token not found")
	})

	t.Run("Delete_Secret", func(t *testing.T) {
		client := openbao.NewClient(apiURL, rootToken)

		// Store a secret
		testPath := "test/delete-me"
		testData := map[string]interface{}{
			"value": "temporary",
		}
		err := client.WriteSecretV2(ctx, "secret", testPath, testData)
		require.NoError(t, err, "Should store secret")

		// Verify it exists
		secret, err := client.ReadSecretV2(ctx, "secret", testPath)
		require.NoError(t, err, "Should retrieve secret")
		require.NotNil(t, secret, "Secret should exist")

		// Delete it
		err = client.DeleteSecretV2(ctx, "secret", testPath)
		require.NoError(t, err, "Should delete secret")

		// Verify it's gone
		secret, err = client.ReadSecretV2(ctx, "secret", testPath)
		assert.Error(t, err, "Should error when reading deleted secret")
		assert.Nil(t, secret, "Secret should be nil")
	})
}

// startOpenBAOContainer starts an OpenBAO container and returns the container, API URL, and root token
func startOpenBAOContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string, string, error) {
	// Run in dev mode - it auto-initializes, unseals, and sets up KV at 'secret'
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/openbao/openbao:latest",
		ExposedPorts: []string{"8200/tcp"},
		Env: map[string]string{
			"OPENBAO_ADDR": "http://0.0.0.0:8200",
			"SKIP_SETCAP":  "true",
		},
		Cmd: []string{"server", "-dev", "-dev-listen-address=0.0.0.0:8200"},
		WaitingFor: wait.ForHTTP("/v1/sys/health").
			WithPort("8200/tcp").
			WithStatusCodeMatcher(func(status int) bool {
				// In dev mode, should return 200 when ready
				return status == 200
			}).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to start container: %w", err)
	}

	// Get the mapped port
	mappedPort, err := container.MappedPort(ctx, "8200")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", "", fmt.Errorf("failed to get host: %w", err)
	}

	apiURL := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	t.Logf("OpenBAO container started at %s", apiURL)

	// Get container logs to extract root token
	time.Sleep(1 * time.Second) // Give it time to output dev mode info
	logs, err := container.Logs(ctx)
	var rootToken string
	if err == nil {
		defer logs.Close()
		buf := new(bytes.Buffer)
		io.Copy(buf, logs)
		logOutput := buf.String()

		// Extract root token from logs using regex
		// Format: "Root Token: s.xxxxxxxxxxxxxxxxxxxxxxxx"
		re := regexp.MustCompile(`Root Token: (s\.[a-zA-Z0-9]+)`)
		matches := re.FindStringSubmatch(logOutput)
		if len(matches) > 1 {
			rootToken = matches[1]
			t.Logf("Extracted root token from logs: %s", rootToken)
		} else {
			t.Logf("WARNING: Could not extract root token from logs")
		}
	}

	if rootToken == "" {
		container.Terminate(ctx)
		return nil, "", "", fmt.Errorf("failed to extract root token from container logs")
	}

	return container, apiURL, rootToken, nil
}
