package auth

import (
	"crypto/rand"
	"encoding/base64"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	accessTokenDuration  = 15 * time.Minute
	refreshTokenDuration = 7 * 24 * time.Hour
	refreshTokenLength   = 32

	jwtIssuer = "playarena"
)

// GenerateAccessToken issues a signed HS256 JWT for the given principal.
// organizationID is empty for platform-level tokens (super-admins).
func GenerateAccessToken(userID, organizationID, role, email, jwtSecret string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:         userID,
		OrganizationID: organizationID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// GenerateRefreshToken returns a cryptographically random, URL-safe base64 token.
// The raw token is never stored; callers must hash it with HashTokenForStorage.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, refreshTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ParseToken validates signature and algorithm; jwt/v5 also validates exp/nbf/iss
// automatically when those fields are present in the token.
func ParseToken(tokenString, jwtSecret string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			// Reject anything that is not HMAC-SHA256.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidAlgorithm
			}
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrInvalidAlgorithm
			}
			return []byte(jwtSecret), nil
		},
		// Enforce issuer validation; tokens from any other service are rejected.
		jwt.WithIssuer(jwtIssuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, mapJWTError(err)
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// Validate custom claims that jwt/v5 cannot enforce.
	if claims.UserID == "" {
		return nil, ErrInvalidToken
	}
	if claims.Email == "" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateToken is the public entry point for access token validation.
func ValidateToken(tokenString, jwtSecret string) (*JWTClaims, error) {
	return ParseToken(tokenString, jwtSecret)
}

// HashTokenForStorage returns the hex-encoded SHA-256 hash of a raw token.
// This is the value stored and queried in the database.
func HashTokenForStorage(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// GetRefreshTokenExpiryTime returns a timestamptz set to now + refresh duration.
func GetRefreshTokenExpiryTime() pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  time.Now().Add(refreshTokenDuration),
		Valid: true,
	}
}

// mapJWTError converts jwt/v5 library errors to our domain error types.
func mapJWTError(err error) error {
	switch {
	case isJWTExpired(err):
		return ErrExpiredToken
	case err == ErrInvalidAlgorithm:
		return ErrInvalidAlgorithm
	default:
		return ErrInvalidToken
	}
}

func isJWTExpired(err error) bool {
	return err != nil && (err.Error() == "token has expired" ||
		containsError(err, jwt.ErrTokenExpired))
}

func containsError(err, target error) bool {
	if err == target {
		return true
	}
	// jwt/v5 wraps errors; unwrap chain
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return containsError(u.Unwrap(), target)
	}
	return false
}
