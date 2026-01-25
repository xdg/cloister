package token

import (
	"regexp"
	"testing"
)

func TestGenerate_Length(t *testing.T) {
	token := Generate()
	if len(token) != 64 {
		t.Errorf("expected token length 64, got %d", len(token))
	}
}

func TestGenerate_HexEncoded(t *testing.T) {
	token := Generate()
	matched, err := regexp.MatchString("^[0-9a-f]{64}$", token)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Errorf("token %q is not 64 lowercase hex characters", token)
	}
}

func TestGenerate_Unique(t *testing.T) {
	const numTokens = 100
	tokens := make(map[string]struct{}, numTokens)

	for i := 0; i < numTokens; i++ {
		token := Generate()
		if _, exists := tokens[token]; exists {
			t.Errorf("duplicate token generated: %s", token)
		}
		tokens[token] = struct{}{}
	}

	if len(tokens) != numTokens {
		t.Errorf("expected %d unique tokens, got %d", numTokens, len(tokens))
	}
}

func TestGenerate_LowercaseHex(t *testing.T) {
	// Generate multiple tokens and ensure all use lowercase hex
	for i := 0; i < 10; i++ {
		token := Generate()
		for _, c := range token {
			if (c >= 'A' && c <= 'F') || (c >= 'G' && c <= 'Z') {
				t.Errorf("token %q contains uppercase character: %c", token, c)
			}
		}
	}
}

func TestTokenBytes_Constant(t *testing.T) {
	if TokenBytes != 32 {
		t.Errorf("expected TokenBytes to be 32, got %d", TokenBytes)
	}
}
