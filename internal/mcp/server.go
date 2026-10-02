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
	"slices"
	"sync"
)

// Protocol versions this server can speak. The server advertises one of these
// (see NewServer) and honours a client that pins the other.
const (
	// ProtocolV20250618 is the classic, stateful revision: initialize
	// handshake, no resultType field.
	ProtocolV20250618 = "2025-06-18"
	// ProtocolV20260728 is the stateless revision: server/discover, a
	// resultType on every result, protocol version carried in _meta.
	ProtocolV20260728 = "2026-07-28"
)

// SupportedProtocolVersions lists every revision the server understands.
var SupportedProtocolVersions = []string{ProtocolV20250618, ProtocolV20260728}

// metaProtocolVersion is the _meta key a 2026-era client uses to carry its
// protocol version on each request instead of an initialize handshake.
const metaProtocolVersion = "io.modelcontextprotocol/protocolVersion"

func isSupportedVersion(v string) bool {
	return slices.Contains(SupportedProtocolVersions, v)
}

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
	// ResultType is set to "complete" only when the negotiated protocol version
	// requires it (2026-07-28+); it stays empty for 2025-06-18 clients.
	ResultType string `json:"resultType,omitempty"`
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

	// defaultVersion is the protocol revision advertised when a client does not
	// pin a supported one. negotiated tracks the revision actually in effect for
	// the current client and decides whether results carry a resultType.
	defaultVersion string
	verMu          sync.RWMutex
	negotiated     string

	out     *bufio.Writer
	writeMu sync.Mutex
}

// NewServer builds a server exposing tools. protocolVersion selects the
// revision the server prefers; it must be one of SupportedProtocolVersions and
// falls back to ProtocolV20250618 when empty or unknown.
func NewServer(name, version, protocolVersion string, tools []Tool) *Server {
	if !isSupportedVersion(protocolVersion) {
		protocolVersion = ProtocolV20250618
	}
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	return &Server{
		name:           name,
		version:        version,
		tools:          tools,
		byName:         byName,
		defaultVersion: protocolVersion,
		negotiated:     protocolVersion,
		out:            bufio.NewWriter(os.Stdout),
	}
}

// setNegotiated records the protocol revision in effect for the client.
func (s *Server) setNegotiated(v string) {
	s.verMu.Lock()
	s.negotiated = v
	s.verMu.Unlock()
}

// isV2026 reports whether the negotiated revision requires 2026-era behaviour
// (a resultType on every result).
func (s *Server) isV2026() bool {
	s.verMu.RLock()
	defer s.verMu.RUnlock()
	return s.negotiated == ProtocolV20260728
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

	// A 2026-era client has no initialize handshake: it pins its protocol
	// version in each request's _meta. Honour it so results are shaped right.
	if v := metaVersion(req.Params); v != "" {
		s.setNegotiated(v)
	}

	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.initializeResult(req.Params))
	case "server/discover":
		s.reply(req.ID, s.discoverResult())
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
	// Honour the client's protocol version when it is one we support; otherwise
	// advertise our configured default. The chosen version is remembered so
	// results are shaped for the right revision.
	version := s.defaultVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(params, &p) == nil && isSupportedVersion(p.ProtocolVersion) {
		version = p.ProtocolVersion
	}
	s.setNegotiated(version)
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

// discoverResult answers server/discover (2026-07-28): it advertises every
// supported protocol version, the server's capabilities, and its identity so a
// client can pick a version up front without an initialize handshake.
func (s *Server) discoverResult() map[string]any {
	return map[string]any{
		"protocolVersions": SupportedProtocolVersions,
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

// metaVersion extracts a supported protocol version pinned in a request's
// _meta, or "" when absent or unsupported.
func metaVersion(params json.RawMessage) string {
	var p struct {
		Meta map[string]any `json:"_meta"`
	}
	if json.Unmarshal(params, &p) != nil {
		return ""
	}
	if v, ok := p.Meta[metaProtocolVersion].(string); ok && isSupportedVersion(v) {
		return v
	}
	return ""
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
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: s.withResultType(result)})
}

// withResultType stamps resultType:"complete" on a result when the negotiated
// revision is 2026-07-28+, where the field is required. For 2025-06-18 it
// returns the result untouched (clients there treat a missing field as
// complete).
func (s *Server) withResultType(result any) any {
	if !s.isV2026() {
		return result
	}
	switch r := result.(type) {
	case Result:
		r.ResultType = "complete"
		return r
	case map[string]any:
		if _, ok := r["resultType"]; !ok {
			r["resultType"] = "complete"
		}
		return r
	default:
		return result
	}
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
