package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

func HashPassword(password string) (string, error) {
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
