package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	tenantKeyPrefix        = "agtk"
	argonTime              = uint32(3)
	argonMemoryKB          = uint32(64 * 1024)
	argonThreads           = uint8(2)
	argonSaltLength        = 16
	argonDerivedKeyLength  = 32
	providerEncryptionNonce = 12
)

type tenantKeyMaterial struct {
	RawKey     string
	KeyID      string
	LookupKey  string
	HashInput  string
	SecretMask string
	KeyFormat  string
}

type presentedTenantKey struct {
	LookupKey string
	HashInput string
}

func newTenantKeyMaterial() (tenantKeyMaterial, error) {
	keyIDBytes, err := randomBytes(6)
	if err != nil {
		return tenantKeyMaterial{}, err
	}
	secretBytes, err := randomBytes(18)
	if err != nil {
		return tenantKeyMaterial{}, err
	}

	keyID := strings.ToUpper(hex.EncodeToString(keyIDBytes))
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	rawKey := fmt.Sprintf("%s_%s_%s", tenantKeyPrefix, keyID, secret)

	return tenantKeyMaterial{
		RawKey:     rawKey,
		KeyID:      keyID,
		LookupKey:  tenantLookupKeyForKeyID(keyID),
		HashInput:  secret,
		SecretMask: maskSecret(rawKey, 12, 4),
		KeyFormat:  "managed_split",
	}, nil
}

func importLegacyTenantKey(rawKey string) tenantKeyMaterial {
	return tenantKeyMaterial{
		RawKey:     rawKey,
		KeyID:      "",
		LookupKey:  tenantLookupKeyForLegacyRaw(rawKey),
		HashInput:  rawKey,
		SecretMask: maskSecret(rawKey, 4, 2),
		KeyFormat:  "legacy_full",
	}
}

func parsePresentedTenantKey(raw string) presentedTenantKey {
	parts := strings.SplitN(raw, "_", 3)
	if len(parts) == 3 && parts[0] == tenantKeyPrefix && strings.TrimSpace(parts[1]) != "" && strings.TrimSpace(parts[2]) != "" {
		return presentedTenantKey{
			LookupKey: tenantLookupKeyForKeyID(strings.ToUpper(strings.TrimSpace(parts[1]))),
			HashInput: strings.TrimSpace(parts[2]),
		}
	}
	return presentedTenantKey{
		LookupKey: tenantLookupKeyForLegacyRaw(raw),
		HashInput: raw,
	}
}

func tenantLookupKeyForKeyID(keyID string) string {
	return "kid:" + strings.ToUpper(strings.TrimSpace(keyID))
}

func tenantLookupKeyForLegacyRaw(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "legacy:" + hex.EncodeToString(sum[:12])
}

func hashSecretSegment(secret string) (string, error) {
	salt, err := randomBytes(argonSaltLength)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(secret), salt, argonTime, argonMemoryKB, argonThreads, argonDerivedKeyLength)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoryKB,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifySecretSegment(encodedHash, secret string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	actualHash := argon2.IDKey([]byte(secret), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

func encryptProviderSecret(masterSecret, plaintext string) (string, error) {
	if strings.TrimSpace(masterSecret) == "" {
		return "", errors.New("secret master key is required")
	}
	key := sha256.Sum256([]byte(masterSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	nonce, err := randomBytes(providerEncryptionNonce)
	if err != nil {
		return "", err
	}
	encrypted := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := append(nonce, encrypted...)
	return base64.RawStdEncoding.EncodeToString(out), nil
}

func decryptProviderSecret(masterSecret, encodedCiphertext string) (string, error) {
	if strings.TrimSpace(masterSecret) == "" {
		return "", errors.New("secret master key is required")
	}
	payload, err := base64.RawStdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(payload) <= providerEncryptionNonce {
		return "", errors.New("ciphertext payload is invalid")
	}
	key := sha256.Sum256([]byte(masterSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, payload[:providerEncryptionNonce], payload[providerEncryptionNonce:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt provider secret: %w", err)
	}
	return string(plaintext), nil
}

func maskProviderSecret(secret string) string {
	return maskSecret(secret, 5, 3)
}

func maskSecret(secret string, prefix, suffix int) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= prefix+suffix {
		if len(secret) <= 4 {
			return strings.Repeat("*", len(secret))
		}
		return secret[:1] + "..." + secret[len(secret)-1:]
	}
	return secret[:prefix] + "..." + secret[len(secret)-suffix:]
}

func randomBytes(size int) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("read random bytes: %w", err)
	}
	return buf, nil
}
