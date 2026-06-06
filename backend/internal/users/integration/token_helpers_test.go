package users_integration_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/auth"
)

const jwtIssuer = "playarena"

// platformAdminToken builds a signed JWT for a platform admin. The
// OrganizationID field is deliberately empty — IsPlatformUser() checks this.
func platformAdminToken(t testing.TB, userID pgtype.UUID, email string) string {
	t.Helper()
	return makeToken(t, uuidStr(userID), "", "platform_admin", email, testJWTSecret)
}

// orgUserToken builds a signed JWT for an org-scoped user with the given role.
func orgUserToken(t testing.TB, userID pgtype.UUID, orgID, role, email string) string {
	t.Helper()
	return makeToken(t, uuidStr(userID), orgID, role, email, testJWTSecret)
}

func makeToken(t testing.TB, userID, orgID, role, email, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return tok
}

// uuidStr formats a pgtype.UUID as the standard hyphenated UUID string.
func uuidStr(u pgtype.UUID) string {
	b := u.Bytes
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}
