package tools

// Helpers for building JSON Schemas equivalent to the zod-to-json-schema output
// the Node server produced for each tool's arguments.

func objSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func strArrayProp(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": desc,
	}
}

// elementProps are the shared {element, ref} arguments used by click/hover/etc.
func elementProps() map[string]any {
	return map[string]any{
		"element": strProp("Human-readable element description used to obtain permission to interact with the element"),
		"ref":     strProp("Exact target element reference from the page snapshot"),
	}
}
