package guardian

// FindTokenForContainer returns the token associated with a container name.
// It calls ListTokens() internally and returns "" on any error (best-effort).
// This is a convenience function that combines token listing and lookup.
func FindTokenForContainer(containerName string) string {
	tokens, err := ListTokens()
	if err != nil {
		return ""
	}
	for tok, name := range tokens {
		if name == containerName {
			return tok
		}
	}
	return ""
}
