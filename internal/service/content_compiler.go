package service

import (
	"bytes"
	"context"
	"errors"
	"html"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/compono/ast"
	"github.com/umono-cms/compono/rule"
	"github.com/umono-cms/umono/internal/models"
)

// quote is the quotation mark of an attribute value.
const quote = "\""

var ErrContentCompilerNotConfigured = errors.New("content compiler is not configured")

type ContentCompiler struct {
	// writeMu serializes changes to globals. Renders do not take it.
	writeMu sync.Mutex
	// globals points to an immutable map of the site global sources by name.
	// A change builds a new map and swaps it in.
	globals atomic.Pointer[map[string]string]

	compono          compono.Compono
	contextProviders []ContextProvider
}

func NewContentCompiler(components []models.Component, contextProviders ...ContextProvider) (*ContentCompiler, error) {
	empty := map[string]string{}
	c := &ContentCompiler{
		compono:          compono.New(),
		contextProviders: contextProviders,
	}
	c.globals.Store(&empty)

	if err := c.LoadGlobalComponents(components); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *ContentCompiler) Compile(source string) (string, []compono.Diagnostic, error) {
	return c.CompileWithProviderContext(context.Background(), source)
}

func (c *ContentCompiler) CompileWithProviderContext(ctx context.Context, source string, extra ...compono.ConvertOption) (string, []compono.Diagnostic, error) {
	globals := *c.globals.Load()
	values, err := c.contextValues(ctx, source, globals)
	if err != nil {
		return "", nil, err
	}
	return c.compile(source, values, globals, extra...)
}

// contextValues returns the context values the conversion of source needs.
func (c *ContentCompiler) contextValues(ctx context.Context, source string, globals map[string]string) (map[string]any, error) {
	if len(c.contextProviders) == 0 {
		return c.convertContext(), nil
	}
	return c.buildCompileContext(ctx, c.contextKeys(source, globals))
}

// RenderRoot renders source for a visitor with its PageRender: the Compono
// output in <umono-root>, with the class of the layout address when the page
// has one. It returns the image URLs Compono wrote to the output, in output
// order, and the diagnostics, which a visitor's response never carries. It is
// the production render that the frozen DOM scenarios call.
func (c *ContentCompiler) RenderRoot(ctx context.Context, source string, r PageRender) (string, []string, []compono.Diagnostic, error) {
	globals := *c.globals.Load()
	values, err := c.contextValues(ctx, source, globals)
	if err != nil {
		return "", nil, nil, err
	}
	var urls []string
	options := append([]compono.ConvertOption{compono.WithURLReporter(func(u compono.URLReport) {
		urls = append(urls, u.URL)
	})}, r.Options()...)
	output, diagnostics, err := c.compile(source, values, globals, options...)
	if err != nil {
		return "", nil, nil, err
	}

	var b strings.Builder
	b.WriteString("<umono-root")
	if class := r.RootClass(); class != "" {
		b.WriteString(" class=" + quote + html.EscapeString(class) + quote)
	}
	b.WriteString(">")
	b.WriteString(output)
	b.WriteString("</umono-root>")
	return b.String(), urls, diagnostics, nil
}

func (c *ContentCompiler) CompileWithContext(source string, values map[string]any) (string, []compono.Diagnostic, error) {
	return c.compile(source, c.convertContext(values), *c.globals.Load())
}

func (c *ContentCompiler) PreviewComponent(name, source string) (string, []compono.Diagnostic, error) {
	return c.PreviewComponentWithProviderContext(context.Background(), name, source)
}

func (c *ContentCompiler) PreviewComponentWithProviderContext(ctx context.Context, name, source string) (string, []compono.Diagnostic, error) {
	name = strings.TrimSpace(name)
	globals := *c.globals.Load()

	if len(c.contextProviders) == 0 {
		return c.previewComponent(name, source, c.convertContext(), globals)
	}

	contextValues, err := c.buildCompileContext(ctx, c.contextKeys(source, globals))
	if err != nil {
		return "", nil, err
	}

	return c.previewComponent(name, source, contextValues, globals)
}

func (c *ContentCompiler) previewComponent(name, source string, contextValues map[string]any, globals map[string]string) (string, []compono.Diagnostic, error) {
	body := []byte(strings.TrimSpace(source))
	var buf bytes.Buffer
	options := append(c.globalOptions(globals, name),
		compono.WithContext(contextValues),
		compono.WithGlobalComponent(name, body),
	)
	diagnostics, err := c.compono.Convert([]byte("{{"+name+"}}"), &buf, options...)
	if err != nil {
		return "", nil, err
	}
	diagnostics = append(diagnostics, requiredBodyDiagnostics(name, body, globals)...)

	return buf.String(), diagnostics, nil
}

func (c *ContentCompiler) LoadGlobalComponents(components []models.Component) error {
	c.updateGlobals(func(globals map[string]string) {
		for _, component := range components {
			globals[component.Name] = component.Content
		}
	})
	return nil
}

func (c *ContentCompiler) LoadGlobalComponent(component models.Component) error {
	c.updateGlobals(func(globals map[string]string) {
		globals[component.Name] = component.Content
	})
	return nil
}

func (c *ContentCompiler) RemoveGlobalComponent(component models.Component) error {
	c.updateGlobals(func(globals map[string]string) {
		delete(globals, component.Name)
	})
	return nil
}

func (c *ContentCompiler) ReloadGlobalComponent(component models.Component) error {
	c.updateGlobals(func(globals map[string]string) {
		globals[component.Name] = component.Content
	})
	return nil
}

// updateGlobals applies change to a copy of the globals and swaps the copy in.
func (c *ContentCompiler) updateGlobals(change func(globals map[string]string)) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	current := *c.globals.Load()
	updated := make(map[string]string, len(current)+1)
	for name, source := range current {
		updated[name] = source
	}
	change(updated)
	c.globals.Store(&updated)
}

func (c *ContentCompiler) compile(source string, context map[string]any, globals map[string]string, extra ...compono.ConvertOption) (string, []compono.Diagnostic, error) {
	var buf bytes.Buffer
	options := append(c.globalOptions(globals, ""), compono.WithContext(context))
	options = append(options, extra...)
	diagnostics, err := c.compono.Convert([]byte(source), &buf, options...)
	if err != nil {
		return "", nil, err
	}

	return buf.String(), diagnostics, nil
}

// globalOptions returns a WithGlobalComponent option for every loaded global
// except skip, sorted by name.
func (c *ContentCompiler) globalOptions(globals map[string]string, skip string) []compono.ConvertOption {
	names := make([]string, 0, len(globals))
	for name := range globals {
		if name == skip {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	options := make([]compono.ConvertOption, 0, len(names))
	for _, name := range names {
		options = append(options, compono.WithGlobalComponent(name, []byte(globals[name])))
	}
	return options
}

func (c *ContentCompiler) buildCompileContext(ctx context.Context, keys []string) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	values := map[string]any{}
	for _, provider := range c.contextProviders {
		if provider == nil {
			continue
		}

		provided, err := provider.BuildCompileContext(ctx, keys)
		if err != nil {
			return nil, err
		}

		for key, value := range provided {
			values[key] = value
		}
	}

	return values, nil
}

func (c *ContentCompiler) contextKeys(source string, globals map[string]string) []string {
	seen := map[string]struct{}{}
	c.collectContextKeys(source, ast.DefaultRootNode(), seen)
	sourceAsGlobal := ast.DefaultEmptyNode()
	sourceAsGlobal.SetRule(rule.NewGlobalCompDef())
	c.collectContextKeys(source, sourceAsGlobal, seen)

	for _, globalSource := range globals {
		node := ast.DefaultEmptyNode()
		node.SetRule(rule.NewGlobalCompDef())
		c.collectContextKeys(globalSource, node, seen)
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ContextKeysUsedBy returns the context keys that source references and that
// the globals it calls reference, following calls between globals, sorted and
// without duplicates. A global that source does not reach is not included.
func (c *ContentCompiler) ContextKeysUsedBy(source string) []string {
	globals := *c.globals.Load()

	seen := map[string]struct{}{}
	c.collectContextKeys(source, ast.DefaultRootNode(), seen)
	sourceAsGlobal := ast.DefaultEmptyNode()
	sourceAsGlobal.SetRule(rule.NewGlobalCompDef())
	c.collectContextKeys(source, sourceAsGlobal, seen)

	visited := map[string]struct{}{}
	queue := []string{source}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		analysis, err := compono.Analyze([]byte(current))
		if err != nil {
			continue
		}

		for _, ref := range analysis.Refs {
			if ref.Local {
				continue
			}
			globalSource, ok := globals[ref.Name]
			if !ok {
				continue
			}
			if _, done := visited[ref.Name]; done {
				continue
			}
			visited[ref.Name] = struct{}{}

			node := ast.DefaultEmptyNode()
			node.SetRule(rule.NewGlobalCompDef())
			c.collectContextKeys(globalSource, node, seen)

			queue = append(queue, globalSource)
		}
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c *ContentCompiler) collectContextKeys(source string, root ast.Node, seen map[string]struct{}) {
	if strings.TrimSpace(source) == "" {
		return
	}

	parsed := c.compono.Parser().Parse([]byte(source), root)
	for _, node := range ast.FilterNodesInTree(parsed, isContextValueNode) {
		key := ast.GetContextKey(node)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
}

func isContextValueNode(node ast.Node) bool {
	return ast.IsRuleNameOneOf(node, []string{
		"context-ref",
		"comp-context-param",
		"comp-call-context-arg",
	})
}

func (c *ContentCompiler) convertContext(values ...map[string]any) map[string]any {
	context := map[string]any{}

	for _, source := range values {
		for key, value := range source {
			context[key] = value
		}
	}

	return context
}
