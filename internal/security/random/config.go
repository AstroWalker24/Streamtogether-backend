package random

// Pre-defined alphabets for use with Generator.String.
const (
	AlphabetUppercase    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	AlphabetLowercase    = "abcdefghijklmnopqrstuvwxyz"
	AlphabetAlpha        = AlphabetUppercase + AlphabetLowercase
	AlphabetNumeric      = "0123456789"
	AlphabetAlphanumeric = AlphabetAlpha + AlphabetNumeric
	AlphabetHex          = "0123456789abcdef"
	AlphabetHexUpper     = "0123456789ABCDEF"
	AlphabetURLSafe      = AlphabetAlphanumeric + "-_"
)
