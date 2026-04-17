package weaver

import "encoding/json"

// Span is one line of a weaver-trace OTLP/JSON file. The JSON shape matches
// `crates/weaver-trace/src/span.rs` in WeaverTools — OTLP field names for
// IDs and timestamps, OpenInference attributes in the `attributes` map.
type Span struct {
	TraceID           string                     `json:"traceId"`
	SpanID            string                     `json:"spanId"`
	ParentSpanID      string                     `json:"parentSpanId,omitempty"`
	Name              string                     `json:"name"`
	StartTimeUnixNano uint64                     `json:"startTimeUnixNano"`
	EndTimeUnixNano   uint64                     `json:"endTimeUnixNano"`
	Attributes        map[string]json.RawMessage `json:"attributes"`
	Status            SpanStatus                 `json:"status"`
	Resource          Resource                   `json:"resource"`
	Scope             Scope                      `json:"scope"`
}

// SpanStatus is the OTel status on a span — Unset/Ok/Error with an optional
// message populated when `error = true` was recorded on the span.
type SpanStatus struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// Resource carries run-level identity replicated onto every span.
//
// Field names follow the OpenTelemetry Resource semantic conventions
// (`service.name`) and weaver-trace's `Resource` struct in
// `crates/weaver-trace/src/span.rs` (`weaver.run_id`,
// `openinference.spec_version`). These are the literal JSON keys the
// Rust side emits — they include dots, not underscores.
type Resource struct {
	ServiceName          string          `json:"service.name"`
	RunID                string          `json:"weaver.run_id"`
	OpenInferenceVersion string          `json:"openinference.spec_version"`
	AdditionalAttributes json.RawMessage `json:"-"`
}

// UnmarshalJSON preserves flattened extra attributes from the Rust side's
// `#[serde(flatten)]` map so host.name and similar show up for display.
func (r *Resource) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["service.name"]; ok {
		_ = json.Unmarshal(v, &r.ServiceName)
		delete(raw, "service.name")
	}
	if v, ok := raw["weaver.run_id"]; ok {
		_ = json.Unmarshal(v, &r.RunID)
		delete(raw, "weaver.run_id")
	}
	if v, ok := raw["openinference.spec_version"]; ok {
		_ = json.Unmarshal(v, &r.OpenInferenceVersion)
		delete(raw, "openinference.spec_version")
	}
	if len(raw) > 0 {
		extra, _ := json.Marshal(raw)
		r.AdditionalAttributes = extra
	}
	return nil
}

// Scope identifies the instrumentation library (weaver-trace + version).
type Scope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AttrString returns the string value of an attribute if present.
func (s *Span) AttrString(key string) string {
	raw, ok := s.Attributes[key]
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(raw, &v); err == nil {
		return v
	}
	return string(raw)
}

// AttrUint64 returns the uint64 value of an attribute if present.
func (s *Span) AttrUint64(key string) uint64 {
	raw, ok := s.Attributes[key]
	if !ok {
		return 0
	}
	var v uint64
	if err := json.Unmarshal(raw, &v); err == nil {
		return v
	}
	return 0
}

// SpanKind returns the OpenInference span kind ("AGENT", "CHAIN", "LLM",
// "TOOL", "EVALUATOR", "RETRIEVER", or "" when unset).
func (s *Span) SpanKind() string {
	return s.AttrString("openinference.span.kind")
}
