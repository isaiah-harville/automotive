// Package flowtypes defines the JSON message contracts passed between
// pipeline vertices (cansource -> udsflasher -> report-formatter ->
// resultsink), so every component -- Go or Python -- agrees on field names.
package flowtypes

// FlashJob is emitted by cansource and consumed by udsflasher.
type FlashJob struct {
	JobID         string `json:"job_id"`
	ECUID         string `json:"ecu_id"`
	MemAddrHex    string `json:"mem_addr_hex"`   // e.g. "00100000"
	FirmwareHex   string `json:"firmware_hex"`   // hex-encoded firmware image
	SecurityLevel byte   `json:"security_level"` // odd request-seed sub-function, default 0x01
	KeyMaskHex    string `json:"key_mask_hex"`   // demo XOR key mask, e.g. "FF"
}

// FlashResult is emitted by udsflasher and consumed downstream by
// report-formatter and resultsink.
type FlashResult struct {
	JobID      string `json:"job_id"`
	ECUID      string `json:"ecu_id"`
	Status     string `json:"status"` // "ok" or "error"
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}
