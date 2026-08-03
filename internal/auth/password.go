package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the plain password.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword returns true if plain matches the hash, false otherwise.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
