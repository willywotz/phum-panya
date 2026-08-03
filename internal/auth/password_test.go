package auth

import "testing"

func TestHashPassword(t *testing.T) {
	plain := "s3cret-pw"
	hash, err := HashPassword(plain)

	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Fatal("hash is empty")
	}

	if hash == plain {
		t.Fatal("hash should not equal plain password")
	}
}

func TestCheckPassword(t *testing.T) {
	plain := "s3cret-pw"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Correct password should return true
	if !CheckPassword(hash, plain) {
		t.Fatal("CheckPassword should return true for correct password")
	}

	// Wrong password should return false
	if CheckPassword(hash, "wrong") {
		t.Fatal("CheckPassword should return false for wrong password")
	}
}
