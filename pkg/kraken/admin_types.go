package kraken

// StatusMessage represents the system status message from Kraken WebSocket API
type StatusMessage struct {
	Channel string       `json:"channel"` // Always "status"
	Type    string       `json:"type"`    // "update"
	Data    []StatusData `json:"data"`
}

// StatusData contains the system status details
type StatusData struct {
	APIVersion   string `json:"api_version"`   // e.g. "v2"
	ConnectionID uint64 `json:"connection_id"` // Unique connection identifier
	System       string `json:"system"`        // "online" when system is available
	Version      string `json:"version"`       // API version number e.g. "2.0.0"
}

type HeartbeatMessage struct {
	Channel string `json:"channel"` // Always "heartbeat"
}

type PinRequestMessage struct {
	Method string `json:"method"` // Always "ping"
}

type PingResponseMessage struct {
	Method   string `json:"method"` // Always "pong"
	Time_in  string `json:"time_in"`
	Time_out string `json:"time_out"`
}
