// Package flowtypes defines the JSON message contracts passed between
// pipeline vertices (cansource -> udsflasher -> report-formatter ->
// resultsink), so every component -- Go or Python -- agrees on field names.
package flowtypes

import "time"

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
	Attempts   int    `json:"attempts"`
}

// FlashDeadLetter is emitted when a flash job cannot be completed. It keeps
// the original job alongside the final result so an operator can inspect and
// replay it after correcting the underlying problem.
type FlashDeadLetter struct {
	Job      FlashJob    `json:"job"`
	Result   FlashResult `json:"result"`
	FailedAt time.Time   `json:"failed_at"`
}
