package random

import (
	"encoding/base64"
	"encoding/hex"
)

// encodeHex returns the lowercase hex encoding of b.
func encodeHex(b []byte) string {
	return hex.EncodeToString(b)
}

// encodeBase64URL returns the unpadded Base64URL encoding of b.
func encodeBase64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
