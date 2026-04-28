package proxy

import "testing"

func TestTenantSecretHashAndVerify(t *testing.T) {
	t.Parallel()

	hash, err := hashSecretSegment("super-secret-value")
	if err != nil {
		t.Fatalf("hashSecretSegment returned error: %v", err)
	}
	if !verifySecretSegment(hash, "super-secret-value") {
		t.Fatal("expected hash verification to succeed")
	}
	if verifySecretSegment(hash, "wrong-value") {
		t.Fatal("expected hash verification to fail for wrong secret")
	}
}

func TestProviderSecretEncryptAndDecrypt(t *testing.T) {
	t.Parallel()

	ciphertext, err := encryptProviderSecret("master-secret", "provider-secret")
	if err != nil {
		t.Fatalf("encryptProviderSecret returned error: %v", err)
	}
	if ciphertext == "provider-secret" {
		t.Fatal("expected ciphertext to differ from plaintext")
	}

	plaintext, err := decryptProviderSecret("master-secret", ciphertext)
	if err != nil {
		t.Fatalf("decryptProviderSecret returned error: %v", err)
	}
	if plaintext != "provider-secret" {
		t.Fatalf("unexpected decrypted plaintext: %q", plaintext)
	}
}
