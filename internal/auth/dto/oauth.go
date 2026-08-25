package dto

// OAuthCallbackInput carries device and session context for the OAuth callback
// handler. Because OAuth callbacks are browser redirects, a JS-generated
// fingerprint is not available; callers should derive one from HTTP metadata or
// leave DeviceFingerprint empty so the service derives it from the provider identity.
type OAuthCallbackInput struct {
	// DeviceFingerprint is an optional client-derived or server-derived device fingerprint.
	// When empty, the Google OAuth service derives one from provider identity + User-Agent.
	DeviceFingerprint string
	// FriendlyName is a human-readable label for the device (e.g. "Chrome on macOS").
	FriendlyName string
	// Platform is the device platform (web/ios/android/desktop).
	Platform string
	// Browser is the browser name parsed from the User-Agent.
	Browser string
	// OS is the operating system parsed from the User-Agent.
	OS string
	// RememberMe controls the refresh token lifetime (extended when true).
	RememberMe bool
}
