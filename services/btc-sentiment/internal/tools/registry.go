package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
)

// Handler executes a named tool.
type Handler func(ctx context.Context, args map[string]any) (any, error)

// Tool is a Gemini function declaration plus its Go handler.
type Tool struct {
	Decl   gemini.FunctionDeclaration
	Handle Handler
	// Terminal tools end the agent loop when called successfully (emit_sentiment).
	Terminal bool
}

// Registry maps tool name → Tool.
type Registry struct {
	order  []string
	byName map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.order = append(r.order, t.Decl.Name)
		r.byName[t.Decl.Name] = t
	}
	return r
}

func (r *Registry) Declarations() []gemini.FunctionDeclaration {
	out := make([]gemini.FunctionDeclaration, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name].Decl)
	}
	return out
}

func (r *Registry) GeminiTools() []gemini.Tool {
	return []gemini.Tool{{FunctionDeclarations: r.Declarations()}}
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

func (r *Registry) Call(ctx context.Context, name string, args map[string]any) (any, bool, error) {
	t, ok := r.byName[name]
	if !ok {
		return nil, false, fmt.Errorf("unknown tool %q", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	res, err := t.Handle(ctx, args)
	return res, t.Terminal, err
}

func argString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("missing %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%q must be string", key)
	}
	return s, nil
}

func argStringSlice(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case []string:
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%q items must be strings", key)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%q must be string array", key)
	}
}

func asJSON(v any) any {
	// Ensure nested structs become JSON-friendly maps for Gemini functionResponse.
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{"raw": string(b)}
	}
	return out
}
