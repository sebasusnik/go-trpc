package router

import "encoding/json"

// Transformer handles serialization/deserialization of tRPC wire formats
// like superjson. Implementations must be safe for concurrent use.
type Transformer interface {
	// TransformInput checks if the raw input uses the transformer envelope format
	// (e.g. {"json": ..., "meta": ...}) and extracts the plain JSON.
	// Returns the plain JSON bytes, whether transformation was applied, and any error.
	TransformInput(raw []byte) ([]byte, bool, error)

	// TransformOutput wraps the output data in the transformer envelope format.
	TransformOutput(data interface{}) (interface{}, error)
}

// superJSONEnvelope is the wire format used by superjson.
type superJSONEnvelope struct {
	JSON json.RawMessage        `json:"json"`
	Meta map[string]interface{} `json:"meta"`
}

// SuperJSONTransformer handles the superjson wire format used by @trpc/client
// when configured with transformer: superjson.
// It auto-detects whether input is in superjson format or plain JSON.
type SuperJSONTransformer struct{}

// TransformInput detects the superjson envelope {"json": ..., "meta": ...}
// and extracts the "json" field. Plain JSON passes through unchanged.
func (t SuperJSONTransformer) TransformInput(raw []byte) ([]byte, bool, error) {
	if len(raw) == 0 {
		return raw, false, nil
	}

	var envelope superJSONEnvelope
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.JSON != nil {
		return []byte(envelope.JSON), true, nil
	}

	return raw, false, nil
}

// TransformOutput wraps the output data in a superjson envelope.
func (t SuperJSONTransformer) TransformOutput(data interface{}) (interface{}, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &superJSONEnvelope{
		JSON: json.RawMessage(jsonBytes),
		Meta: map[string]interface{}{},
	}, nil
}
