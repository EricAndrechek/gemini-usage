package gemini

// Model variant -> quota bucket mapping.
// Pro variants are exclusive to the Pro bucket.
// Flash and Thinking share many variant IDs; we use the presence of
// "thoughts" in the response to distinguish them.

var proOnlyVariants = map[string]bool{
	"9d8ca3786ebdfbea": true, // Pro bucket top-level
	"d1f674dda82d1455": true,
	"e5a44cb1dae2b489": true,
	"4d79521e1e77dd3b": true,
	"b1e46a6037e6aa9f": true,
	"0e0f3a3749fc6a5c": true,
	"6cb69cd4b6cae77d": true,
	"e6fa609c3fa255c0": true, // gemini-3.1-pro (most common)
	"852fc722e6249d28": true,
}

// classifyTurn determines the quota bucket for a turn.
func classifyTurn(modelID string, hasThoughts bool) string {
	if proOnlyVariants[modelID] {
		return "pro"
	}
	if hasThoughts {
		return "thinking"
	}
	return "flash"
}
