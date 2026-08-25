package dto

// MessageResponse is a simple acknowledgment for operations with no data to return.
type MessageResponse struct {
	Message string `json:"message"`
}
