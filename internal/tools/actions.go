package tools

import (
	"context"
	"encoding/json"

	"github.com/ngxuanth/mcp-server/internal/bridge"
	"github.com/ngxuanth/mcp-server/internal/mcp"
)

// --- Navigation (these return a fresh page snapshot) ---

func navigate(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_navigate",
		Description: "Navigate to a URL",
		InputSchema: objSchema(map[string]any{
			"url": strProp("The URL to navigate to"),
		}, "url"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			if _, err := forward(ctx, hub, "browser_navigate", args); err != nil {
				return mcp.Result{}, err
			}
			return snapshotResult(ctx, hub)
		},
	}
}

func goBack(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_go_back",
		Description: "Go back to the previous page",
		InputSchema: objSchema(map[string]any{}),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			if _, err := hub.Send(ctx, "browser_go_back", map[string]any{}); err != nil {
				return mcp.Result{}, err
			}
			return snapshotResult(ctx, hub)
		},
	}
}

func goForward(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_go_forward",
		Description: "Go forward to the next page",
		InputSchema: objSchema(map[string]any{}),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			if _, err := hub.Send(ctx, "browser_go_forward", map[string]any{}); err != nil {
				return mcp.Result{}, err
			}
			return snapshotResult(ctx, hub)
		},
	}
}

func snapshot(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_snapshot",
		Description: "Capture accessibility snapshot of the current page. Use this for getting references to elements to interact with.",
		InputSchema: objSchema(map[string]any{}),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			return snapshotResult(ctx, hub)
		},
	}
}

// snapshotResult captures the page and returns it as a single text result.
func snapshotResult(ctx context.Context, hub sender) (mcp.Result, error) {
	snap, err := captureAriaSnapshot(ctx, hub)
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Text(snap), nil
}

// --- Interactions (these append a fresh snapshot after acting) ---

func click(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_click",
		Description: "Perform click on a web page",
		InputSchema: objSchema(elementProps(), "element", "ref"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Element string `json:"element"`
			}
			_ = json.Unmarshal(args, &a)
			return actWithSnapshot(ctx, hub, "browser_click", args, "Clicked "+quote(a.Element))
		},
	}
}

func drag(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_drag",
		Description: "Perform drag and drop between two elements",
		InputSchema: objSchema(map[string]any{
			"startElement": strProp("Human-readable source element description used to obtain the permission to interact with the element"),
			"startRef":     strProp("Exact source element reference from the page snapshot"),
			"endElement":   strProp("Human-readable target element description used to obtain the permission to interact with the element"),
			"endRef":       strProp("Exact target element reference from the page snapshot"),
		}, "startElement", "startRef", "endElement", "endRef"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				StartElement string `json:"startElement"`
				EndElement   string `json:"endElement"`
			}
			_ = json.Unmarshal(args, &a)
			return actWithSnapshot(ctx, hub, "browser_drag", args,
				"Dragged "+quote(a.StartElement)+" to "+quote(a.EndElement))
		},
	}
}

func hover(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_hover",
		Description: "Hover over element on page",
		InputSchema: objSchema(elementProps(), "element", "ref"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Element string `json:"element"`
			}
			_ = json.Unmarshal(args, &a)
			return actWithSnapshot(ctx, hub, "browser_hover", args, "Hovered over "+quote(a.Element))
		},
	}
}

func typeText(hub *bridge.Hub) mcp.Tool {
	props := elementProps()
	props["text"] = strProp("Text to type into the element")
	props["submit"] = boolProp("Whether to submit entered text (press Enter after)")
	return mcp.Tool{
		Name:        "browser_type",
		Description: "Type text into editable element",
		InputSchema: objSchema(props, "element", "ref", "text", "submit"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(args, &a)
			return actWithSnapshot(ctx, hub, "browser_type", args, "Typed "+quote(a.Text))
		},
	}
}

func selectOption(hub *bridge.Hub) mcp.Tool {
	props := elementProps()
	props["values"] = strArrayProp("Array of values to select in the dropdown. This can be a single value or multiple values.")
	return mcp.Tool{
		Name:        "browser_select_option",
		Description: "Select an option in a dropdown",
		InputSchema: objSchema(props, "element", "ref", "values"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Element string `json:"element"`
			}
			_ = json.Unmarshal(args, &a)
			return actWithSnapshot(ctx, hub, "browser_select_option", args, "Selected option in "+quote(a.Element))
		},
	}
}

// actWithSnapshot performs an action, then returns the status line followed by a
// fresh page snapshot, mirroring the Node interaction tools.
func actWithSnapshot(ctx context.Context, hub *bridge.Hub, msgType string, args json.RawMessage, status string) (mcp.Result, error) {
	if _, err := forward(ctx, hub, msgType, args); err != nil {
		return mcp.Result{}, err
	}
	snap, err := captureAriaSnapshot(ctx, hub)
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Result{Content: []mcp.Content{
		{Type: "text", Text: status},
		{Type: "text", Text: snap},
	}}, nil
}

// --- Common ---

func pressKey(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_press_key",
		Description: "Press a key on the keyboard",
		InputSchema: objSchema(map[string]any{
			"key": strProp("Name of the key to press or a character to generate, such as `ArrowLeft` or `a`"),
		}, "key"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Key string `json:"key"`
			}
			_ = json.Unmarshal(args, &a)
			if _, err := forward(ctx, hub, "browser_press_key", args); err != nil {
				return mcp.Result{}, err
			}
			return mcp.Text("Pressed key " + a.Key), nil
		},
	}
}

func wait(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_wait",
		Description: "Wait for a specified time in seconds",
		InputSchema: objSchema(map[string]any{
			"time": numProp("The time to wait in seconds"),
		}, "time"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Time json.Number `json:"time"`
			}
			_ = json.Unmarshal(args, &a)
			if _, err := forward(ctx, hub, "browser_wait", args); err != nil {
				return mcp.Result{}, err
			}
			return mcp.Text("Waited for " + a.Time.String() + " seconds"), nil
		},
	}
}

// --- Custom ---

func getConsoleLogs(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_get_console_logs",
		Description: "Get the console logs from the browser",
		InputSchema: objSchema(map[string]any{}),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			raw, err := hub.Send(ctx, "browser_get_console_logs", map[string]any{})
			if err != nil {
				return mcp.Result{}, err
			}
			var logs []json.RawMessage
			_ = json.Unmarshal(raw, &logs)
			text := ""
			for i, l := range logs {
				if i > 0 {
					text += "\n"
				}
				text += string(l)
			}
			return mcp.Text(text), nil
		},
	}
}

func screenshot(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_screenshot",
		Description: "Take a screenshot of the current page",
		InputSchema: objSchema(map[string]any{}),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			data, err := sendString(ctx, hub, "browser_screenshot", map[string]any{})
			if err != nil {
				return mcp.Result{}, err
			}
			return mcp.Result{Content: []mcp.Content{
				{Type: "image", Data: data, MimeType: "image/png"},
			}}, nil
		},
	}
}

func evaluate(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_evaluate",
		Description: "Evaluate a JavaScript expression in the page and return its JSON-serialisable value. Promises are awaited. Use it to read state (text, attributes, upload progress) without taking a full snapshot.",
		InputSchema: objSchema(map[string]any{
			"expression": strProp("JavaScript expression, e.g. document.title or (() => { ... })()"),
		}, "expression"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			raw, err := forward(ctx, hub, "browser_evaluate", args)
			if err != nil {
				return mcp.Result{}, err
			}
			if len(raw) == 0 {
				return mcp.Text("undefined"), nil
			}
			return mcp.Text(string(raw)), nil
		},
	}
}
