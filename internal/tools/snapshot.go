package tools

import (
	"context"
	"encoding/json"
)

// captureAriaSnapshot fetches the URL, title and accessibility snapshot of the
// page and formats them like the Node server's captureAriaSnapshot.
func captureAriaSnapshot(ctx context.Context, hub sender) (string, error) {
	url, err := sendString(ctx, hub, "getUrl", nil)
	if err != nil {
		return "", err
	}
	title, err := sendString(ctx, hub, "getTitle", nil)
	if err != nil {
		return "", err
	}
	snap, err := sendString(ctx, hub, "browser_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return "\n- Page URL: " + url +
		"\n- Page Title: " + title +
		"\n- Page Snapshot\n```yaml\n" + snap + "\n```\n", nil
}

// sendString sends a message and decodes the result as a string. Non-string
// results (numbers, objects) fall back to their raw JSON text.
func sendString(ctx context.Context, hub sender, msgType string, payload any) (string, error) {
	raw, err := hub.Send(ctx, msgType, payload)
	if err != nil {
		return "", err
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	return string(raw), nil
}

// quote wraps a value in double quotes, matching the Node server's result text
// (e.g. Clicked "the button").
func quote(s string) string {
	return "\"" + s + "\""
}
