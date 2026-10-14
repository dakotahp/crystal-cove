package server

import (
	"context"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dakotahp/crystal-cove/internal/search"
	"github.com/dakotahp/crystal-cove/internal/vault"
)

// addTool registers a tool and records the parameters it takes, which
// argumentHints quotes when a call passes arguments that do not fit.
func addTool[In, Out any](srv *mcp.Server, params map[string]string, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if values, ok := toolChoices[t.Name]; ok {
		if schema, err := jsonschema.For[In](nil); err == nil {
			for name, choices := range values {
				if p := schema.Properties[name]; p != nil {
					for _, c := range choices {
						p.Enum = append(p.Enum, c)
					}
				}
			}
			t.InputSchema = schema
		}
	}
	mcp.AddTool(srv, t, h)
	params[t.Name] = describeParams[In]()
}

// toolChoices lists the values each string parameter that picks among fixed
// options accepts. They go into the advertised schema as an enum, so a client
// shows the options and rejects any other value before calling.
var toolChoices = map[string]map[string][]string{
	"search_notes": {
		"mode":   {"auto", string(search.ModeWords), string(search.ModePhrase), string(search.ModeRegex)},
		"output": {outputLines, outputFiles},
	},
	"list_notes":   {"order": {"asc", "desc"}},
	"edit_section": {"mode": {string(vault.SectionAppend), string(vault.SectionPrepend), string(vault.SectionReplace)}},
}

func describeParams[In any]() string {
	schema, err := jsonschema.For[In](nil)
	if err != nil || len(schema.Properties) == 0 {
		return "no parameters"
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	slices.Sort(names)
	out := "parameters: " + strings.Join(names, ", ")
	if len(schema.Required) > 0 {
		out += " (required: " + strings.Join(schema.Required, ", ") + ")"
	}
	return out
}

// argumentHints adds a tool's parameter names to the SDK's argument
// validation error, which names only the offending argument, so a model
// that guessed a parameter name can correct the call in one retry.
func argumentHints(params map[string]string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			call, ok := req.(*mcp.CallToolRequest)
			if !ok {
				return res, err
			}
			result, ok := res.(*mcp.CallToolResult)
			if !ok || result == nil || !result.IsError || len(result.Content) == 0 {
				return res, err
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if ok && strings.HasPrefix(text.Text, `validating "arguments"`) {
				text.Text += ". " + call.Params.Name + " takes " + params[call.Params.Name]
			}
			return res, err
		}
	}
}
