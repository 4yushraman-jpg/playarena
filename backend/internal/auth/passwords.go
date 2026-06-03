package auth

import "golang.org/x/crypto/bcrypt"

const (
	bcryptCost = 12

	// maxPasswordBytes is the maximum UTF-8 byte length accepted by HashPassword.
	//
	// bcrypt truncates input at exactly 72 bytes. Without this guard a password
	// of 73+ bytes is silently hashed as its first 72 bytes, meaning any other
	// string sharing the same first 72 bytes would authenticate successfully.
	// Rejecting oversized input before hashing eliminates that ambiguity.
	//
	// The validate tag on RegisterRequest.Password enforces max=72 by rune count,
	// which is sufficient for ASCII passwords. This byte-level check is the
	// authoritative backstop for passwords containing multi-byte Unicode characters
	// (where rune count ≤ 72 but byte length may exceed 72).
	maxPasswordBytes = 72
)

func HashPassword(password string) (string, error) {
	if len([]byte(password)) > maxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// dummyBcryptHash is a pre-computed cost-12 bcrypt hash used when a login
// attempt references an email address that does not exist. Performing a real
// bcrypt comparison—even against this dummy value—equalises the response time
// between "email not found" and "wrong password", preventing timing-based
// email enumeration attacks.
//
// Generated once at process startup; bcrypt cost 12 matches the real cost.
var dummyBcryptHash string

func init() {
	h, err := bcrypt.GenerateFromPassword(
		[]byte("playarena:timing-equalization:do-not-use"),
		bcryptCost,
	)
	if err != nil {
		panic("auth: failed to compute timing-equalization bcrypt hash: " + err.Error())
	}
	dummyBcryptHash = string(h)
}
