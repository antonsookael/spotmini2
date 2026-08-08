package backend

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// generateCodeVerifier produces the random secret PKCE uses in place
// of a client secret: it's created fresh for each login, never leaves
// this machine, and only its hash (the "challenge") is sent to
// Spotify up front - the verifier itself is sent later, at token
// exchange, so Spotify can confirm the code exchange came from
// whoever started this specific login.
func generateCodeVerifier() (string, error) {
	return randomURLSafe()
}

// randomURLSafe returns 32 bytes of cryptographically random data as an
// unpadded base64url string - the shape both the PKCE verifier and the
// OAuth state value need, neither of which may be guessable.
func randomURLSafe() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// codeChallengeFor hashes a PKCE verifier the way Spotify expects
// (S256): base64url(sha256(verifier)), no padding.
func codeChallengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
