package lotevent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const MaxBody = 256 * 1024

// LotEvent is a signed inbound lot / step / tool notification.
type LotEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func Parse(body []byte) (LotEvent, error) {
	if len(body) == 0 {
		return LotEvent{}, fmt.Errorf("empty body")
	}
	if len(body) > MaxBody {
		return LotEvent{}, fmt.Errorf("body %d exceeds %d bytes", len(body), MaxBody)
	}
	var env LotEvent
	if err := json.Unmarshal(body, &env); err != nil {
		return LotEvent{}, fmt.Errorf("json: %v", err)
	}
	if err := ValidateKind(env.Type); err != nil {
		return LotEvent{}, err
	}
	if len(env.Payload) == 0 || string(env.Payload) == "null" {
		return LotEvent{}, fmt.Errorf("payload is required")
	}
	if !json.Valid(env.Payload) {
		return LotEvent{}, fmt.Errorf("payload is not valid json")
	}
	if err := validatePayload(env.Type, env.Payload); err != nil {
		return LotEvent{}, err
	}
	return env, nil
}

type Payload struct {
	LotID          string `json:"lot_id"`
	ToolID         string `json:"tool_id"`
	StepID         string `json:"step_id"`
	ProcessProgram string `json:"process_program"`
	WaferCount     int    `json:"wafer_count"`
	CarrierID      string `json:"carrier_id"`
	HoldCode       string `json:"hold_code"`
	AlarmCode      string `json:"alarm_code"`
}

func validatePayload(kind string, raw json.RawMessage) error {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	switch kind {
	case "lot.track_in", "lot.track_out", "step.start", "step.complete":
		if strings.TrimSpace(p.LotID) == "" {
			return fmt.Errorf("lot_id is required")
		}
		if strings.TrimSpace(p.ToolID) == "" {
			return fmt.Errorf("tool_id is required")
		}
		if strings.TrimSpace(p.StepID) == "" {
			return fmt.Errorf("step_id is required")
		}
		if strings.TrimSpace(p.ProcessProgram) == "" {
			return fmt.Errorf("process_program is required")
		}
		if p.WaferCount < 0 || p.WaferCount > 25 {
			return fmt.Errorf("wafer_count must be in [0,25]")
		}
	case "lot.hold", "lot.release":
		if strings.TrimSpace(p.LotID) == "" {
			return fmt.Errorf("lot_id is required")
		}
	case "tool.alarm":
		if strings.TrimSpace(p.ToolID) == "" {
			return fmt.Errorf("tool_id is required")
		}
	}
	return nil
}

func ValidateKind(t string) error {
	t = strings.TrimSpace(t)
	if t == "" {
		return fmt.Errorf("event type is required")
	}
	if len(t) > 128 {
		return fmt.Errorf("event type too long")
	}
	parts := strings.Split(t, ".")
	if len(parts) < 1 {
		return fmt.Errorf("event type is empty")
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("event type has empty segment")
		}
		for _, r := range p {
			if unicode.IsUpper(r) {
				return fmt.Errorf("event type must be lowercase")
			}
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
			if !ok {
				return fmt.Errorf("event type has illegal character %q", r)
			}
		}
	}
	return nil
}

func MatchPrefix(eventType, prefix string) bool {
	if prefix == "" {
		return true
	}
	if prefix == eventType {
		return true
	}
	if strings.HasSuffix(prefix, ".") {
		return strings.HasPrefix(eventType, prefix)
	}
	return eventType == prefix || strings.HasPrefix(eventType, prefix+".")
}

func KnownKinds() []string {
	return []string{
		"lot.track_in",
		"lot.track_out",
		"step.start",
		"step.complete",
		"lot.hold",
		"lot.release",
		"tool.alarm",
	}
}
