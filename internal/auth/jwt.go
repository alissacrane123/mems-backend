// Every Go file starts with a package declaration.
// This tells Go this file belongs to the "auth" package.
package auth

import (
	"errors"
	"os"
	"time"

	// This is a third-party JWT library we installed earlier with go get
	"github.com/golang-jwt/jwt/v5"
)

// This is a struct - think of it like a TypeScript interface or a class.
// It defines the shape of data we store inside the JWT token.
// jwt.RegisteredClaims is embedded here (like extending a class) -
// it adds standard JWT fields like ExpiresAt and IssuedAt automatically.
type Claims struct {
	UserID string `json:"user_id"` // the backtick tags tell Go how to name these fields in JSON
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT token for a given user.
// In Go, functions can return multiple values - here we return
// the token string AND an error (nil if everything went fine).
func GenerateToken(userID, email string) (string, error) {
	// os.Getenv reads a value from your .env file (loaded at startup)
	secret := os.Getenv("JWT_SECRET")

	// Create the claims (payload) that will be encoded inside the token
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			// Token expires 7 days from now
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Create the token using HMAC-SHA256 signing method with our claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and return the token as a string using our secret key.
	// []byte() converts the string secret to bytes, which is what the library expects.
	return token.SignedString([]byte(secret))
}

// ValidateToken takes a JWT string and checks if it's valid.
// If valid, it returns the Claims so we can read the user's ID and email.
// If invalid or expired, it returns an error.
func ValidateToken(tokenStr string) (*Claims, error) {
	secret := os.Getenv("JWT_SECRET")

	// ParseWithClaims decodes the token and validates the signature.
	// The third argument is a callback function that returns the secret key -
	// it also lets us double-check the signing method for security.
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Make sure the token is using HMAC (not some other algorithm).
		// This prevents a known attack where someone swaps the algorithm to "none".
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})

	// In Go, it's idiomatic to check errors immediately after the call that might fail.
	// err != nil means something went wrong.
	if err != nil {
		return nil, err
	}

	// Type-assert the token claims back into our Claims struct.
	// The "ok" variable tells us if the assertion succeeded.
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Return the claims and nil for the error (nil error = success in Go)
	return claims, nil
}