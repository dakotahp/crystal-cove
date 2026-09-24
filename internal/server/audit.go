package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// auditedArgs are the only tool arguments the audit log records. Note text,
// search queries and edit text stay out: the log says who touched which
// note, never what the note says.
var auditedArgs = []string{"vault", "path", "new_path", "to", "dir"}

// maxAuditedArg caps a recorded argument, so a client cannot flood the log.
const maxAuditedArg = 256

// SetAuditLog records every tool call to log. Call it before serving.
func (s *Server) SetAuditLog(log *slog.Logger) { s.audit = log }

func auditMiddleware(log *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			toolCall, ok := req.(*mcp.CallToolRequest)
			if !ok {
				return next(ctx, method, req)
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			attrs := []any{
				"tool", toolCall.Params.Name,
				"caller", caller(toolCall.Extra),
				"outcome", outcome(res, err),
				"duration_ms", time.Since(start).Milliseconds(),
			}
			log.Info("tool call", append(attrs, auditArgs(toolCall.Params.Arguments)...)...)
			return res, err
		}
	}
}

// caller names who made a request: the OIDC subject, or api-key for the
// static token, which identifies no one in particular.
func caller(extra *mcp.RequestExtra) string {
	if extra == nil || extra.TokenInfo == nil {
		return "unknown"
	}
	if extra.TokenInfo.UserID == "" {
		return "api-key"
	}
	return extra.TokenInfo.UserID
}

func outcome(res mcp.Result, err error) string {
	if err != nil {
		return "error"
	}
	if r, ok := res.(*mcp.CallToolResult); ok && r.IsError {
		return "tool_error"
	}
	return "ok"
}

func auditArgs(raw json.RawMessage) []any {
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil {
		return nil
	}
	var attrs []any
	for _, key := range auditedArgs {
		v, ok := args[key].(string)
		if !ok {
			continue
		}
		if len(v) > maxAuditedArg {
			v = v[:maxAuditedArg] + "..."
		}
		attrs = append(attrs, key, v)
	}
	return attrs
}
