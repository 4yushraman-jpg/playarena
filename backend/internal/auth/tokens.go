package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	accessTokenDuration        = 15 * time.Minute
	refreshTokenDuration       = 7 * 24 * time.Hour
	verificationTokenDuration  = 24 * time.Hour
	passwordResetTokenDuration = 1 * time.Hour

	// randomTokenLength is the number of raw bytes used for both refresh tokens
	// and email verification tokens (base64url-encoded to ~43 chars).
	randomTokenLength = 32

	jwtIssuer = "playarena"
)

// GenerateAccessToken issues a signed HS256 JWT for the given principal.
// organizationID is empty for platform/player/onboarding tokens.
// scope is the explicit persona scope (player|organizer|onboarding|platform).
// playerProfileID is set only for player-scope tokens.
func GenerateAccessToken(userID, organizationID, role, email, scope, playerProfileID, jwtSecret string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:          userID,
		OrganizationID:  organizationID,
		Role:            role,
		Email:           email,
		Scope:           scope,
		PlayerProfileID: playerProfileID,
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
	return generateRandomToken()
}

// GenerateVerificationToken returns a cryptographically random, URL-safe base64
// token for email verification. Same entropy and encoding as refresh tokens.
// The raw token is never stored; callers must hash it with HashTokenForStorage.
func GenerateVerificationToken() (string, error) {
	return generateRandomToken()
}

// generateRandomToken is the shared implementation for all random-token
// generation. Generates randomTokenLength bytes of CSPRNG output and
// encodes them as base64url.
func generateRandomToken() (string, error) {
	b := make([]byte, randomTokenLength)
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
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidAlgorithm
			}
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrInvalidAlgorithm
			}
			return []byte(jwtSecret), nil
		},
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

// GetVerificationTokenExpiryTime returns a timestamptz set to now + 24 hours.
func GetVerificationTokenExpiryTime() pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  time.Now().Add(verificationTokenDuration),
		Valid: true,
	}
}

// GetPasswordResetTokenExpiryTime returns a timestamptz set to now + 1 hour.
// Password reset windows are intentionally short to limit replay exposure.
func GetPasswordResetTokenExpiryTime() pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  time.Now().Add(passwordResetTokenDuration),
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
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return containsError(u.Unwrap(), target)
	}
	return false
}
