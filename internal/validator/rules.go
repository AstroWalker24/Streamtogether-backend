package validator

// fieldMessages maps go-playground/validator tag names to human-readable templates.
// %s is substituted with the human-formatted field name at call time.
var fieldMessages = map[string]string{
	"required":         "%s is required.",
	"email":            "%s must be a valid email address.",
	"uuid":             "%s must be a valid UUID.",
	"uuid4":            "%s must be a valid UUID v4.",
	"url":              "%s must be a valid URL.",
	"http_url":         "%s must be a valid HTTP URL.",
	"min":              "%s is too short.",
	"max":              "%s is too long.",
	"gte":              "%s must be greater than or equal to the minimum value.",
	"lte":              "%s must be less than or equal to the maximum value.",
	"gt":               "%s must be greater than the minimum value.",
	"lt":               "%s must be less than the maximum value.",
	"len":              "%s must have the exact required length.",
	"oneof":            "%s must be one of the allowed values.",
	"alphanum":         "%s must contain only letters and numbers.",
	"alpha":            "%s must contain only letters.",
	"numeric":          "%s must be a numeric value.",
	"datetime":         "%s must be a valid date/time.",
	"boolean":          "%s must be a boolean value.",
	"hostname":         "%s must be a valid hostname.",
	"hostname_rfc1123": "%s must be a valid hostname.",
	"ip":               "%s must be a valid IP address.",
	"ipv4":             "%s must be a valid IPv4 address.",
	"ipv6":             "%s must be a valid IPv6 address.",
	"json":             "%s must be valid JSON.",
	"base64":           "%s must be valid Base64.",
	"e164":             "%s must be a valid E.164 phone number.",
	"printascii":       "%s must contain only printable ASCII characters.",
	"ascii":            "%s must contain only ASCII characters.",
	"contains":         "%s must contain the required value.",
	"excludes":         "%s must not contain the excluded value.",
	"startswith":       "%s must start with the required prefix.",
	"endswith":         "%s must end with the required suffix.",
	"unique":           "%s must contain unique values.",
}

// messageFor returns the human-readable message template for a validator tag.
// Falls back to a generic message if the tag is not registered.
func messageFor(tag string) string {
	if msg, ok := fieldMessages[tag]; ok {
		return msg
	}
	return "%s is invalid."
}
