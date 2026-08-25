package dto

// ResetPasswordRequest is the inbound DTO for the password-reset flow.
// Password equality is enforced in the service layer.
type ResetPasswordRequest struct {
	Token           string `json:"token"            validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required"`
}
