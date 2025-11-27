package secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the service name used in the OS keyring
	KeyringService = "foundry"
	// KeyringUser is the user/account name for the OpenBAO token
	KeyringUser = "openbao-token"
	// FallbackFileName is the filename for fallback file storage
	FallbackFileName = ".foundry-token"
)

// StoreAuthToken stores the OpenBAO auth token in the OS keyring
// Falls back to file storage if keyring is unavailable
func StoreAuthToken(token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Try to store in OS keyring first
	err := keyring.Set(KeyringService, KeyringUser, token)
	if err == nil {
		return nil
	}

	// If keyring fails, fall back to file storage
	return storeTokenInFile(token)
}

// LoadAuthToken retrieves the OpenBAO auth token from the OS keyring
// Falls back to file storage if keyring is unavailable
func LoadAuthToken() (string, error) {
	// Try to load from OS keyring first
	token, err := keyring.Get(KeyringService, KeyringUser)
	if err == nil {
		return token, nil
	}

	// If keyring fails, fall back to file storage
	return loadTokenFromFile()
}

// ClearAuthToken removes the OpenBAO auth token from storage
func ClearAuthToken() error {
	// Try to delete from keyring
	keyringErr := keyring.Delete(KeyringService, KeyringUser)

	// Also try to delete from file (in case it was stored there)
	fileErr := deleteTokenFile()

	// If both fail, return an error
	if keyringErr != nil && fileErr != nil {
		return fmt.Errorf("failed to clear token from keyring (%v) and file (%v)", keyringErr, fileErr)
	}

	return nil
}

// storeTokenInFile stores the token in a file with restrictive permissions
func storeTokenInFile(token string) error {
	tokenPath, err := getTokenFilePath()
	if err != nil {
		return fmt.Errorf("failed to get token file path: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write token to file with restrictive permissions (0600 = read/write for owner only)
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// loadTokenFromFile loads the token from file storage
func loadTokenFromFile() (string, error) {
	tokenPath, err := getTokenFilePath()
	if err != nil {
		return "", fmt.Errorf("failed to get token file path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		return "", fmt.Errorf("no auth token found in keyring or file storage")
	}

	// Read token from file
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	return string(data), nil
}

// deleteTokenFile removes the token file if it exists
func deleteTokenFile() error {
	tokenPath, err := getTokenFilePath()
	if err != nil {
		return fmt.Errorf("failed to get token file path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		// File doesn't exist, nothing to delete
		return nil
	}

	// Delete the file
	if err := os.Remove(tokenPath); err != nil {
		return fmt.Errorf("failed to delete token file: %w", err)
	}

	return nil
}

// getTokenFilePath returns the path to the token file
// Respects FOUNDRY_CONFIG_DIR environment variable if set
func getTokenFilePath() (string, error) {
	// Check if there's an override via environment variable
	if dir := os.Getenv("FOUNDRY_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, FallbackFileName), nil
	}

	// Default to ~/.foundry/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".foundry", FallbackFileName), nil
}
