package ssh

import (
	"crypto/ed25519"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)
	require.NotNil(t, kp)

	t.Run("private key is PEM encoded", func(t *testing.T) {
		assert.NotEmpty(t, kp.Private)

		// Should be PEM encoded
		block, _ := pem.Decode(kp.Private)
		require.NotNil(t, block, "private key should be PEM encoded")
		assert.Equal(t, "OPENSSH PRIVATE KEY", block.Type)

		// Should be a valid Ed25519 key
		assert.Equal(t, ed25519.PrivateKeySize, len(block.Bytes))
	})

	t.Run("public key is OpenSSH format", func(t *testing.T) {
		assert.NotEmpty(t, kp.Public)

		publicKeyStr := string(kp.Public)

		// OpenSSH public keys start with the algorithm
		assert.True(t, strings.HasPrefix(publicKeyStr, "ssh-ed25519 "),
			"public key should start with 'ssh-ed25519 '")

		// Should end with a newline
		assert.True(t, strings.HasSuffix(publicKeyStr, "\n"),
			"public key should end with newline")

		// Should be parseable
		_, _, _, _, err := ssh.ParseAuthorizedKey(kp.Public)
		assert.NoError(t, err, "public key should be parseable")
	})

	t.Run("public and private keys match", func(t *testing.T) {
		// Parse the private key
		signer, err := kp.ParsePrivateKey()
		require.NoError(t, err)

		// Get the public key from the signer
		publicKey := signer.PublicKey()

		// Parse the stored public key
		storedPublicKey, _, _, _, err := ssh.ParseAuthorizedKey(kp.Public)
		require.NoError(t, err)

		// They should match
		assert.Equal(t, ssh.MarshalAuthorizedKey(storedPublicKey),
			ssh.MarshalAuthorizedKey(publicKey))
	})

	t.Run("each generation produces unique keys", func(t *testing.T) {
		kp2, err := GenerateKeyPair()
		require.NoError(t, err)

		// Keys should be different
		assert.NotEqual(t, kp.Private, kp2.Private)
		assert.NotEqual(t, kp.Public, kp2.Public)
	})
}

func TestKeyPair_PublicKeyString(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	pubKeyStr := kp.PublicKeyString()

	assert.NotEmpty(t, pubKeyStr)
	assert.True(t, strings.HasPrefix(pubKeyStr, "ssh-ed25519 "))
	assert.Equal(t, string(kp.Public), pubKeyStr)
}

func TestKeyPair_PrivateKeyPEM(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	privateKeyPEM := kp.PrivateKeyPEM()

	assert.NotEmpty(t, privateKeyPEM)
	assert.Equal(t, kp.Private, privateKeyPEM)

	// Should be valid PEM
	block, _ := pem.Decode(privateKeyPEM)
	assert.NotNil(t, block)
}

func TestKeyPair_ParsePrivateKey(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	t.Run("valid private key", func(t *testing.T) {
		signer, err := kp.ParsePrivateKey()
		require.NoError(t, err)
		assert.NotNil(t, signer)

		// Signer should have a public key
		publicKey := signer.PublicKey()
		assert.NotNil(t, publicKey)

		// Public key type should be ssh-ed25519
		assert.Equal(t, "ssh-ed25519", publicKey.Type())
	})

	t.Run("invalid PEM data", func(t *testing.T) {
		invalidKP := &KeyPair{
			Private: []byte("not valid PEM data"),
			Public:  kp.Public,
		}

		_, err := invalidKP.ParsePrivateKey()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode PEM block")
	})

	t.Run("empty private key", func(t *testing.T) {
		emptyKP := &KeyPair{
			Private: []byte{},
			Public:  kp.Public,
		}

		_, err := emptyKP.ParsePrivateKey()
		assert.Error(t, err)
	})
}

func TestKeyPair_AuthMethod(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	t.Run("valid auth method", func(t *testing.T) {
		authMethod, err := kp.AuthMethod()
		require.NoError(t, err)
		assert.NotNil(t, authMethod)
	})

	t.Run("invalid private key returns error", func(t *testing.T) {
		invalidKP := &KeyPair{
			Private: []byte("not valid PEM data"),
			Public:  kp.Public,
		}

		_, err := invalidKP.AuthMethod()
		assert.Error(t, err)
	})
}

func TestKeyPair_EndToEnd(t *testing.T) {
	// Generate a key pair
	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	// Get auth method
	authMethod, err := kp.AuthMethod()
	require.NoError(t, err)

	// Verify we can create connection options with it
	opts := DefaultConnectionOptions("example.com", "testuser", authMethod)
	err = opts.Validate()
	assert.NoError(t, err)
}

func TestEncodePrivateKeyToPEM(t *testing.T) {
	// Generate a key pair
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	assert.NotNil(t, publicKey)

	pemData, err := encodePrivateKeyToPEM(privateKey)
	require.NoError(t, err)
	assert.NotEmpty(t, pemData)

	// Decode and verify
	block, _ := pem.Decode(pemData)
	require.NotNil(t, block)
	assert.Equal(t, "OPENSSH PRIVATE KEY", block.Type)
	assert.Equal(t, ed25519.PrivateKeySize, len(block.Bytes))
}

func TestFormatPublicKeyOpenSSH(t *testing.T) {
	// Generate a key pair
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	formatted, err := formatPublicKeyOpenSSH(publicKey)
	require.NoError(t, err)
	assert.NotEmpty(t, formatted)

	// Should be parseable
	parsedKey, _, _, _, err := ssh.ParseAuthorizedKey(formatted)
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519", parsedKey.Type())
}
