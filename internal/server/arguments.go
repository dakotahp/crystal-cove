package server

import (
	"context"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addTool registers a tool and records the parameters it takes, which
// argumentHints quotes when a call passes arguments that do not fit.
func addTool[In, Out any](srv *mcp.Server, params map[string]string, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if schema := enumSchema[In](); schema != nil {
		t.InputSchema = schema
	}
	mcp.AddTool(srv, t, h)
	params[t.Name] = describeParams[In]()
}

// enumSchema infers In's input schema with each field's enum struct tag
// applied, which jsonschema-go does not read, so clients see the allowed
// values and reject any other. It is nil when no field has the tag.
func enumSchema[In any]() *jsonschema.Schema {
	typ := reflect.TypeFor[In]()
	if typ.Kind() != reflect.Struct {
		return nil
	}
	enums := map[string][]any{}
	for i := range typ.NumField() {
		field := typ.Field(i)
		values, ok := field.Tag.Lookup("enum")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		for _, v := range strings.Split(values, ",") {
			enums[name] = append(enums[name], v)
		}
	}
	if len(enums) == 0 {
		return nil
	}
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil
	}
	for name, values := range enums {
		schema.Properties[name].Enum = values
	}
	return schema
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
