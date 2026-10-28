package adminapi

import "encoding/json"

const MaxFrameSize = 1 << 20

type MigrationStartParams struct {
	Kind            string `json:"kind,omitempty"`
	IncludeAddons   *bool  `json:"include_addons,omitempty"`
	OutputDirectory string `json:"output_directory"`
}
type MigrationJobParams struct {
	JobID string `json:"job_id"`
}

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}
type Response struct {
	ID     string    `json:"id"`
	Result any       `json:"result,omitempty"`
	Error  *APIError `json:"error,omitempty"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type AccountLabelsParams struct {
	AccountID   string `json:"account_id"`
	PersonLabel string `json:"person_label"`
	EmailLabel  string `json:"email_label"`
}
type EnrollmentParams struct {
	Role        string `json:"role,omitempty"`
	PersonLabel string `json:"person_label"`
	EmailLabel  string `json:"email_label"`
}
type EnrollmentCompleteParams struct {
	EnrollmentID string `json:"enrollment_id"`
}
type ReassignParams struct {
	ConsoleXUID string `json:"console_xuid"`
	AccountID   string `json:"account_id"`
}
type PlayerParams struct {
	ConsoleXUID string `json:"console_xuid"`
}
