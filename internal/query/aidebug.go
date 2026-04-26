package query

type TraceRecord struct {
	TS        string `json:"ts"`
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	TurnIndex int    `json:"turn_index,omitempty"`
	Seq       int    `json:"seq,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

type TraceEmitter interface {
	Emit(record TraceRecord)
}

type AIDebugConfig struct {
	Enabled bool
	Emitter TraceEmitter
}
