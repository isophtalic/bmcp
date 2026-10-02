// Package mcp implements a minimal Model Context Protocol server over stdio
// (newline-delimited JSON-RPC 2.0). It mirrors the behaviour of the Node
// @modelcontextprotocol/sdk server used by the original project: list tools,
// call a tool, and report tool failures as results with isError rather than as
// protocol errors.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
)

// Content is a single piece of a tool result (text or image).
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// Result is what a tool returns to the client.
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Text is a convenience constructor for a single text result.
func Text(s string) Result {
	return Result{Content: []Content{{Type: "text", Text: s}}}
}

// Tool is a callable exposed to the client.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handle      func(ctx context.Context, args json.RawMessage) (Result, error)
}

// Server speaks MCP over stdin/stdout.
type Server struct {
	name    string
	version string
	tools   []Tool
	byName  map[string]Tool

	out     *bufio.Writer
	writeMu sync.Mutex
}

// NewServer builds a server exposing tools.
func NewServer(name, version string, tools []Tool) *Server {
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	return &Server{
		name:    name,
		version: version,
		tools:   tools,
		byName:  byName,
		out:     bufio.NewWriter(os.Stdout),
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from stdin until EOF or ctx is cancelled. Each request
// is handled in its own goroutine so a slow tool call does not block others;
// writes to stdout are serialised.
func (s *Server) Serve(ctx context.Context) error {
	reader := bufio.NewReaderSize(os.Stdin, 1<<20)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var req rpcRequest
			if json.Unmarshal(line, &req) == nil && req.Method != "" {
				wg.Add(1)
				go func() {
					defer wg.Done()
					s.dispatch(ctx, req)
				}()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) {
	// A request without an id is a notification: act on it but never reply.
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.initializeResult(req.Params))
	case "notifications/initialized", "notifications/cancelled":
		// Nothing to do.
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, s.listToolsResult())
	case "tools/call":
		s.reply(req.ID, s.callTool(ctx, req.Params))
	case "resources/list":
		s.reply(req.ID, map[string]any{"resources": []any{}})
	case "resources/templates/list":
		s.reply(req.ID, map[string]any{"resourceTemplates": []any{}})
	case "prompts/list":
		s.reply(req.ID, map[string]any{"prompts": []any{}})
	default:
		if !isNotification {
			s.replyError(req.ID, -32601, "Method not found: "+req.Method)
		}
	}
}

func (s *Server) initializeResult(params json.RawMessage) map[string]any {
	// Echo the client's protocol version when provided, for maximum
	// compatibility; otherwise advertise a recent one.
	version := "2025-06-18"
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    s.name,
			"version": s.version,
		},
	}
}

func (s *Server) listToolsResult() map[string]any {
	list := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		list = append(list, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return map[string]any{"tools": list}
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) Result {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return errorResult("Invalid tool call: " + err.Error())
	}
	tool, ok := s.byName[call.Name]
	if !ok {
		return errorResult("Tool \"" + call.Name + "\" not found")
	}
	result, err := tool.Handle(ctx, call.Arguments)
	if err != nil {
		return errorResult(err.Error())
	}
	return result
}

func errorResult(msg string) Result {
	return Result{Content: []Content{{Type: "text", Text: msg}}, IsError: true}
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return // Notification: no response.
	}
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) replyError(id json.RawMessage, code int, message string) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) write(resp rpcResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.out.Write(data)
	s.out.WriteByte('\n')
	s.out.Flush()
}
