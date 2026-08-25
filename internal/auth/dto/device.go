package dto

// RenameDeviceRequest is the inbound DTO for the rename-device operation.
type RenameDeviceRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}
