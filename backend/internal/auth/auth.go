package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// NOTE: This is a self-contained JWT + bcrypt auth so the app runs with zero
// AWS setup. To move to Amazon Cognito later, replace IssueToken/ParseToken
// with Cognito Hosted UI + a JWKS verifier, and drop password_hash from users.
// The rest of the app only depends on the (UserID, OrgID) pulled from context.

type Claims struct {
	UserID uuid.UUID `json:"uid"`
	OrgID  uuid.UUID `json:"org"`
	jwt.RegisteredClaims
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func IssueToken(secret string, userID, orgID uuid.UUID) (string, error) {
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}
