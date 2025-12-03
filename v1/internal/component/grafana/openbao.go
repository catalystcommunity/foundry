package grafana

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const (
	// OpenBAO path constants for Grafana secrets
	openBAOMount        = "foundry-core"
	openBAOSecretPath   = "grafana"
	openBAOPasswordKey  = "admin_password"
	openBAOUsernameKey  = "admin_user"
)

// OpenBAOClient defines the interface for storing/retrieving secrets in OpenBAO
type OpenBAOClient interface {
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
}

// StoreGrafanaCredentials stores the Grafana admin credentials in OpenBAO
func StoreGrafanaCredentials(ctx context.Context, client OpenBAOClient, username, password string) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Check if credentials already exist and match
	existing, err := client.ReadSecretV2(ctx, openBAOMount, openBAOSecretPath)
	if err == nil && existing != nil {
		existingPassword, _ := existing[openBAOPasswordKey].(string)
		existingUsername, _ := existing[openBAOUsernameKey].(string)
		if existingPassword == password && existingUsername == username {
			// Credentials already exist and match
			return nil
		}
	}

	// Store the credentials
	secretData := map[string]interface{}{
		openBAOPasswordKey: password,
		openBAOUsernameKey: username,
	}

	return client.WriteSecretV2(ctx, openBAOMount, openBAOSecretPath, secretData)
}

// GetGrafanaCredentials retrieves the Grafana admin credentials from OpenBAO
func GetGrafanaCredentials(ctx context.Context, client OpenBAOClient) (username, password string, err error) {
	if client == nil {
		return "", "", fmt.Errorf("OpenBAO client is required")
	}

	data, err := client.ReadSecretV2(ctx, openBAOMount, openBAOSecretPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Grafana credentials from OpenBAO: %w", err)
	}

	if data == nil {
		return "", "", fmt.Errorf("Grafana credentials not found in OpenBAO")
	}

	password, ok := data[openBAOPasswordKey].(string)
	if !ok || password == "" {
		return "", "", fmt.Errorf("Grafana admin password not found or empty in OpenBAO")
	}

	username, _ = data[openBAOUsernameKey].(string)
	if username == "" {
		username = "admin" // Default username
	}

	return username, password, nil
}

// GetGrafanaPassword retrieves only the Grafana admin password from OpenBAO
func GetGrafanaPassword(ctx context.Context, client OpenBAOClient) (string, error) {
	_, password, err := GetGrafanaCredentials(ctx, client)
	return password, err
}

// EnsureGrafanaCredentials ensures Grafana credentials exist in OpenBAO
// If they exist, returns them. If not, generates a new password, stores it, and returns it.
func EnsureGrafanaCredentials(ctx context.Context, client OpenBAOClient, username, providedPassword string) (string, string, error) {
	if client == nil {
		return "", "", fmt.Errorf("OpenBAO client is required")
	}

	// Try to read existing credentials
	existingUsername, existingPassword, err := GetGrafanaCredentials(ctx, client)
	if err == nil && existingPassword != "" {
		return existingUsername, existingPassword, nil
	}

	// Generate password if not provided
	password := providedPassword
	if password == "" {
		password, err = generateSecurePassword(32)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate password: %w", err)
		}
	}

	// Use provided username or default
	if username == "" {
		username = "admin"
	}

	// Store the credentials
	if err := StoreGrafanaCredentials(ctx, client, username, password); err != nil {
		return "", "", fmt.Errorf("failed to store credentials: %w", err)
	}

	return username, password, nil
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 and truncate to desired length
	password := base64.URLEncoding.EncodeToString(bytes)
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}
