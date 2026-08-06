package mcp

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

// maxHTTPBody bounds an incoming JSON-RPC message (GPX imports can be large).
const maxHTTPBody = 8 << 20

// NewHTTPHandler serves the Garmin MCP tool set over the streamable-HTTP
// transport. resolve authenticates the request and returns the Garmin client
// to serve it with; returning an error rejects the request with 401. This is
// the multi-user hook: the host application maps its own credentials (bearer
// token, session…) to a per-user *garmin.Client.
func NewHTTPHandler(resolve func(*http.Request) (*garmin.Client, error)) http.Handler {
	return NewHTTPServerHandler(func(r *http.Request) (*Server, error) {
		client, err := resolve(r)
		if err != nil {
			return nil, err
		}
		return NewServer(client), nil
	})
}

// NewHTTPServerHandler serves an arbitrary MCP Server over the streamable-HTTP
// transport: each POST carries one JSON-RPC message and gets its JSON-RPC
// response back (notifications are acknowledged with 202 and no body). The
// transport is stateless — no session is kept between calls, which is all the
// tools/call workflow needs.
//
// resolve authenticates the request and returns the Server (any tool set —
// see NewToolServer) handling it; returning an error rejects the request with
// 401.
func NewHTTPServerHandler(resolve func(*http.Request) (*Server, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed (MCP streamable HTTP: POST one JSON-RPC message)", http.StatusMethodNotAllowed)
			return
		}
		s, err := resolve(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxHTTPBody))
		if err != nil {
			http.Error(w, "reading body", http.StatusBadRequest)
			return
		}
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeRPC(w, http.StatusBadRequest, errorResponse(nil, codeParseError, "parse error", err.Error()))
			return
		}

		result, rerr := s.dispatch(r.Context(), req)

		// A request without an id is a notification: acknowledge, no body.
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
		if rerr != nil {
			resp = rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rerr}
		}
		writeRPC(w, http.StatusOK, resp)
	})
}

func writeRPC(w http.ResponseWriter, status int, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
