package openbao

import (
	"context"
	"fmt"
)

// InitRequest represents the request body for initializing OpenBAO
type InitRequest struct {
	SecretShares    int `json:"secret_shares"`
	SecretThreshold int `json:"secret_threshold"`
}

// InitResponse represents the response from initializing OpenBAO
type InitResponse struct {
	Keys       []string `json:"keys"`
	KeysBase64 []string `json:"keys_base64"`
	RootToken  string   `json:"root_token"`
}

// Initialize initializes a new OpenBAO instance
func (c *Client) Initialize(ctx context.Context, shares, threshold int) (*InitResponse, error) {
	if shares < 1 {
		return nil, fmt.Errorf("secret_shares must be at least 1")
	}
	if threshold < 1 {
		return nil, fmt.Errorf("secret_threshold must be at least 1")
	}
	if threshold > shares {
		return nil, fmt.Errorf("secret_threshold (%d) cannot exceed secret_shares (%d)", threshold, shares)
	}

	req := InitRequest{
		SecretShares:    shares,
		SecretThreshold: threshold,
	}

	resp, err := c.doRequest(ctx, "PUT", "/v1/sys/init", req)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}

	var initResp InitResponse
	if err := readResponse(resp, &initResp); err != nil {
		return nil, fmt.Errorf("failed to read init response: %w", err)
	}

	return &initResp, nil
}

// UnsealRequest represents the request body for unsealing OpenBAO
type UnsealRequest struct {
	Key   string `json:"key"`
	Reset bool   `json:"reset,omitempty"`
}

// UnsealResponse represents the response from an unseal operation
type UnsealResponse struct {
	Sealed       bool   `json:"sealed"`
	T            int    `json:"t"`
	N            int    `json:"n"`
	Progress     int    `json:"progress"`
	Nonce        string `json:"nonce"`
	Version      string `json:"version"`
	ClusterName  string `json:"cluster_name"`
	ClusterID    string `json:"cluster_id"`
	Type         string `json:"type"`
	RecoverySeal bool   `json:"recovery_seal"`
}

// Unseal submits an unseal key to OpenBAO
func (c *Client) Unseal(ctx context.Context, key string) (*UnsealResponse, error) {
	if key == "" {
		return nil, fmt.Errorf("unseal key is required")
	}

	req := UnsealRequest{
		Key: key,
	}

	resp, err := c.doRequest(ctx, "PUT", "/v1/sys/unseal", req)
	if err != nil {
		return nil, fmt.Errorf("failed to unseal: %w", err)
	}

	var unsealResp UnsealResponse
	if err := readResponse(resp, &unsealResp); err != nil {
		return nil, fmt.Errorf("failed to read unseal response: %w", err)
	}

	return &unsealResp, nil
}

// UnsealWithKeys unseals OpenBAO using the provided keys (submits until unsealed)
func (c *Client) UnsealWithKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("at least one unseal key is required")
	}

	for i, key := range keys {
		resp, err := c.Unseal(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to unseal with key %d: %w", i+1, err)
		}

		// If unsealed, we're done
		if !resp.Sealed {
			return nil
		}

		// If we've used all keys and still sealed, something is wrong
		if i == len(keys)-1 && resp.Sealed {
			return fmt.Errorf("unsealing failed: still sealed after %d keys (need %d of %d)", resp.Progress, resp.T, resp.N)
		}
	}

	return nil
}

// ResetUnseal resets the unseal progress
func (c *Client) ResetUnseal(ctx context.Context) error {
	req := UnsealRequest{
		Reset: true,
	}

	resp, err := c.doRequest(ctx, "PUT", "/v1/sys/unseal", req)
	if err != nil {
		return fmt.Errorf("failed to reset unseal: %w", err)
	}

	var unsealResp UnsealResponse
	if err := readResponse(resp, &unsealResp); err != nil {
		return fmt.Errorf("failed to read reset response: %w", err)
	}

	return nil
}

// VerifySealed checks if OpenBAO is currently sealed
func (c *Client) VerifySealed(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check health: %w", err)
	}

	return health.IsSealed(), nil
}

// VerifyInitialized checks if OpenBAO has been initialized
func (c *Client) VerifyInitialized(ctx context.Context) (bool, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check health: %w", err)
	}

	return health.IsInitialized(), nil
}
