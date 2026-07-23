package component

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type Renderer interface {
	Render(context.Context, json.RawMessage) (string, error)
}

type RendererFunc func(context.Context, json.RawMessage) (string, error)

func (f RendererFunc) Render(ctx context.Context, props json.RawMessage) (string, error) {
	return f(ctx, props)
}

// EntityRenderer 是需要按稳定 Entity/Claim 读取动态数据的可信渲染器。
// displayConfig 已由 ComponentVersion 的 props schema 校验。
type EntityRenderer interface {
	RenderEntity(context.Context, uuid.UUID, json.RawMessage) (string, error)
}

// Registry maps persisted renderer refs to trusted in-process implementations.
// Database values never load code or templates dynamically.
type Registry struct {
	mu        sync.RWMutex
	renderers map[string]Renderer
}

func NewRegistry() *Registry {
	return &Registry{renderers: make(map[string]Renderer)}
}

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	if err := registry.Register("builtin.key_value", RendererFunc(renderKeyValue)); err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Register(ref string, renderer Renderer) error {
	ref = strings.TrimSpace(ref)
	if ref == "" || renderer == nil {
		return ErrInvalidDefinition
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.renderers[ref]; exists {
		return fmt.Errorf("%w: %s", ErrRendererRegistered, ref)
	}
	r.renderers[ref] = renderer
	return nil
}

func (r *Registry) Has(ref string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.renderers[ref]
	return exists
}

func (r *Registry) Render(ctx context.Context, ref string, props json.RawMessage) (string, error) {
	r.mu.RLock()
	renderer, exists := r.renderers[ref]
	r.mu.RUnlock()
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrRendererNotFound, ref)
	}
	return renderer.Render(ctx, props)
}

// RenderEntity 优先调用动态 Entity 渲染器；普通组件仍只消费 display_config。
func (r *Registry) RenderEntity(
	ctx context.Context, ref string, entityID uuid.UUID, displayConfig json.RawMessage,
) (string, error) {
	r.mu.RLock()
	renderer, exists := r.renderers[ref]
	r.mu.RUnlock()
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrRendererNotFound, ref)
	}
	if dynamic, ok := renderer.(EntityRenderer); ok {
		return dynamic.RenderEntity(ctx, entityID, displayConfig)
	}
	return renderer.Render(ctx, displayConfig)
}

func renderKeyValue(_ context.Context, props json.RawMessage) (string, error) {
	var values map[string]any
	if err := json.Unmarshal(props, &values); err != nil {
		return "", fmt.Errorf("%w: props must be an object", ErrInvalidProps)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString(`<dl data-component-renderer="key-value">`)
	for _, key := range keys {
		value, err := componentValueText(values[key])
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "<dt>%s</dt><dd>%s</dd>",
			html.EscapeString(key), html.EscapeString(value))
	}
	out.WriteString("</dl>")
	return out.String(), nil
}

func componentValueText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidProps, err)
	}
	return string(encoded), nil
}
