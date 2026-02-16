package token //nolint:revive // intentional: does not conflict at import path level

import (
	"crypto/rand"
	"encoding/hex"
)

// TokenBytes is the number of random bytes used for token generation.
// 32 bytes = 256 bits of entropy, which provides strong security.
const TokenBytes = 32

// Generate creates a new cryptographically secure random token.
// The token is 32 random bytes encoded as 64 lowercase hex characters.
func Generate() string {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read should never fail on modern systems.
		// If it does, it indicates a critical system failure.
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
