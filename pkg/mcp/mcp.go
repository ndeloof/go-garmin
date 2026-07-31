// Package mcp exposes a go-garmin client as a Model Context Protocol server
// over stdio, inspired by github.com/taxuspt/garmin_mcp. It speaks JSON-RPC
// 2.0 with newline-delimited messages (the MCP stdio transport) and, like the
// rest of go-garmin, depends only on the standard library.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

// protocolVersion is the MCP revision this server implements; the client's
// requested version is echoed back when present.
const protocolVersion = "2025-06-18"

const serverName = "go-garmin"

// ServerVersion is reported to MCP clients in the initialize response.
var ServerVersion = "0.1.0"

// Server serves a Garmin Connect client over the MCP stdio transport.
type Server struct {
	client *garmin.Client
	tools  []tool
	index  map[string]tool
}

// Tool is one MCP tool: a name, a human description, a JSON Schema for its
// arguments, and a handler that runs it against the Garmin client.
type tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Handler     func(ctx context.Context, args json.RawMessage) (any, error)
}

// NewServer builds an MCP server exposing the Garmin client's data.
func NewServer(client *garmin.Client) *Server {
	s := &Server{client: client, index: map[string]tool{}}
	s.register(garminTools(client))
	return s
}

func (s *Server) register(tools []tool) {
	for _, t := range tools {
		s.tools = append(s.tools, t)
		s.index[t.Name] = t
	}
}

// JSON-RPC 2.0 envelopes.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// ServeStdio reads newline-delimited JSON-RPC messages from r and writes
// responses to w until r is exhausted or ctx is cancelled. Notifications
// (requests without an id) are handled without a reply.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	br := bufio.NewReaderSize(r, 1<<20)
	enc := json.NewEncoder(w)
	var writeMu sync.Mutex
	send := func(resp rpcResponse) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = enc.Encode(resp) // json.Encoder writes the trailing newline
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			s.handleLine(ctx, line, send)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *Server) handleLine(ctx context.Context, line []byte, send func(rpcResponse)) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		send(errorResponse(nil, codeParseError, "parse error", err.Error()))
		return
	}
	// A request without an id is a notification: process, never reply.
	notification := len(req.ID) == 0

	result, rerr := s.dispatch(ctx, req)
	if notification {
		return
	}
	if rerr != nil {
		send(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rerr})
		return
	}
	send(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.Params), nil
	case "notifications/initialized", "notifications/cancelled", "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.toolList()}, nil
	case "tools/call":
		return s.handleToolCall(ctx, req.Params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

func (s *Server) handleInitialize(params json.RawMessage) any {
	version := protocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": ServerVersion},
	}
}

func (s *Server) toolList() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
	}
	return out
}

func (s *Server) handleToolCall(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid tools/call params: " + err.Error()}
	}
	t, ok := s.index[call.Name]
	if !ok {
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown tool: " + call.Name}
	}
	result, err := t.Handler(ctx, call.Arguments)
	if err != nil {
		// Tool execution errors are reported in-band so the model sees them.
		return toolResult(fmt.Sprintf("error: %v", err), true), nil
	}
	text, err := marshalText(result)
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	return toolResult(text, false), nil
}

// toolResult builds an MCP tool result with a single text content block.
func toolResult(text string, isError bool) map[string]any {
	res := map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
	if isError {
		res["isError"] = true
	}
	return res
}

// marshalText renders a handler result as indented JSON text. Raw JSON is
// re-indented for readability, falling back to the original bytes.
func marshalText(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		var tmp any
		if json.Unmarshal(raw, &tmp) == nil {
			if b, err := json.MarshalIndent(tmp, "", "  "); err == nil {
				return string(b), nil
			}
		}
		return string(raw), nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func errorResponse(id json.RawMessage, code int, msg, data string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg, Data: data}}
}
