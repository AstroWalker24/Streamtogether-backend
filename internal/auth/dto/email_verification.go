package dto

// VerifyEmailRequest is the inbound DTO for the email verification flow.
// Token is the raw plaintext token extracted from the verification link.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// ResendVerificationRequest is the inbound DTO for re-sending a verification email.
type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}
