package model

// ProtectionEvent is one user-visible SRP lifecycle operation.
type ProtectionEvent struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}
