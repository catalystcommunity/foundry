package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// GenerateKeyPair generates an Ed25519 SSH key pair
// Ed25519 is preferred over RSA because it's:
// - More secure with smaller keys (256-bit vs 2048/4096-bit)
// - Faster to generate and use
// - More resistant to side-channel attacks
func GenerateKeyPair() (*KeyPair, error) {
	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Encode private key to PEM format
	privateKeyPEM, err := encodePrivateKeyToPEM(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}

	// Format public key in OpenSSH authorized_keys format
	publicKeyBytes, err := formatPublicKeyOpenSSH(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to format public key: %w", err)
	}

	return &KeyPair{
		Private: privateKeyPEM,
		Public:  publicKeyBytes,
	}, nil
}

// encodePrivateKeyToPEM encodes an Ed25519 private key to PEM format
func encodePrivateKeyToPEM(privateKey ed25519.PrivateKey) ([]byte, error) {
	// For Ed25519, we use PEM encoding with the raw key
	// The golang.org/x/crypto/ssh package doesn't directly support marshaling
	// private keys to the full OpenSSH format, so we use a simplified PEM approach
	block := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: privateKey,
	}

	return pem.EncodeToMemory(block), nil
}

// formatPublicKeyOpenSSH formats an Ed25519 public key in OpenSSH authorized_keys format
func formatPublicKeyOpenSSH(publicKey ed25519.PublicKey) ([]byte, error) {
	// Convert to ssh.PublicKey
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	// Marshal to OpenSSH authorized_keys format
	return ssh.MarshalAuthorizedKey(sshPublicKey), nil
}

// PublicKeyString returns the public key as an OpenSSH authorized_keys formatted string
func (kp *KeyPair) PublicKeyString() string {
	return string(kp.Public)
}

// PrivateKeyPEM returns the private key in PEM format
func (kp *KeyPair) PrivateKeyPEM() []byte {
	return kp.Private
}

// ParsePrivateKey parses a PEM-encoded private key and returns an ssh.Signer
func (kp *KeyPair) ParsePrivateKey() (ssh.Signer, error) {
	// Decode PEM block
	block, _ := pem.Decode(kp.Private)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Parse the private key
	privateKey := ed25519.PrivateKey(block.Bytes)

	// Create signer
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}

	return signer, nil
}

// AuthMethod returns an ssh.AuthMethod that can be used for authentication
func (kp *KeyPair) AuthMethod() (ssh.AuthMethod, error) {
	signer, err := kp.ParsePrivateKey()
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}
