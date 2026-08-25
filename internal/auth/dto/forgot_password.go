package dto

// ForgotPasswordRequest is the inbound DTO for the forgot-password flow.
// The response is always a generic success regardless of whether the email exists.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}
