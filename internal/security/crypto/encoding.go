package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// HexEncode returns the lowercase hex encoding of b.
func HexEncode(b []byte) string {
	return hex.EncodeToString(b)
}

// HexDecode decodes a hex string into bytes.
func HexDecode(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecodeFailed, err)
	}
	return b, nil
}

// Base64URLEncode returns the unpadded Base64URL encoding of b.
func Base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// Base64URLDecode decodes an unpadded Base64URL string into bytes.
func Base64URLDecode(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecodeFailed, err)
	}
	return b, nil
}
