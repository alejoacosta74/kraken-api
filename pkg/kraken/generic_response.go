package kraken

import "encoding/json"

type GenericResponse struct {
	Channel string          `json:"channel,omitempty"`
	Method  string          `json:"method,omitempty"`
	Type    string          `json:"type,omitempty"`
	Error   string          `json:"error,omitempty"`
	Success bool            `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}
