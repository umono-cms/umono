package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/umono-cms/umono/internal/models"
)

func TestContentCompilerLoadsGlobalComponents(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "# Hello"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.Compile("{{ GREETING }}")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if strings.TrimSpace(output) != "<h1>Hello</h1>" {
		t.Fatalf("Compile() = %q", output)
	}
}

func TestContentCompilerPreviewComponent(t *testing.T) {
	compiler, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.PreviewComponent("GREETING", "# Hello")
	if err != nil {
		t.Fatalf("PreviewComponent() error = %v", err)
	}

	if strings.TrimSpace(output) != "<h1>Hello</h1>" {
		t.Fatalf("PreviewComponent() = %q", output)
	}
}

func TestContentCompilerRemovesGlobalComponent(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "# Hello"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	if err := compiler.RemoveGlobalComponent(models.Component{Name: "GREETING"}); err != nil {
		t.Fatalf("RemoveGlobalComponent() error = %v", err)
	}

	output, _, err := compiler.Compile("{{ GREETING }}")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if strings.TrimSpace(output) != "" {
		t.Fatalf("Compile() = %q", output)
	}
}

func TestContentCompilerReloadsGlobalComponent(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "# Hello"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	if err := compiler.ReloadGlobalComponent(models.Component{Name: "GREETING", Content: "# Hi"}); err != nil {
		t.Fatalf("ReloadGlobalComponent() error = %v", err)
	}

	output, _, err := compiler.Compile("{{ GREETING }}")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if strings.TrimSpace(output) != "<h1>Hi</h1>" {
		t.Fatalf("Compile() = %q", output)
	}
}

func TestContentCompilerPreviewComponentReplacesLoadedGlobal(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "# Hello"},
		{Name: "SITE_NAME", Content: "# Site"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.PreviewComponent("GREETING", "{{ SITE_NAME }}\n\n# Hi")
	if err != nil {
		t.Fatalf("PreviewComponent() error = %v", err)
	}

	if strings.TrimSpace(output) != "<h1>Site</h1><h1>Hi</h1>" {
		t.Fatalf("PreviewComponent() = %q", output)
	}
}

func TestContentCompilerCompilesWhileGlobalsChange(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "# Hello"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(10)

	for g := 0; g < 8; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				output, _, err := compiler.Compile("{{ GREETING }}")
				if err != nil {
					t.Errorf("Compile() error = %v", err)
					continue
				}
				trimmed := strings.TrimSpace(output)
				if trimmed != "<h1>Hello</h1>" && trimmed != "<h1>Hi</h1>" {
					t.Errorf("Compile() = %q", output)
				}
			}
		}()
	}

	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			content := "# Hi"
			if i%2 != 0 {
				content = "# Hello"
			}
			if err := compiler.ReloadGlobalComponent(models.Component{Name: "GREETING", Content: content}); err != nil {
				t.Errorf("ReloadGlobalComponent() error = %v", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := compiler.LoadGlobalComponent(models.Component{Name: fmt.Sprintf("EXTRA_%d", i), Content: "# Extra"}); err != nil {
				t.Errorf("LoadGlobalComponent() error = %v", err)
			}
		}
	}()

	wg.Wait()

	for i := 0; i < 20; i++ {
		output, _, err := compiler.Compile(fmt.Sprintf("{{ EXTRA_%d }}", i))
		if err != nil {
			t.Fatalf("Compile() error = %v", err)
		}
		if strings.TrimSpace(output) != "<h1>Extra</h1>" {
			t.Fatalf("Compile() = %q", output)
		}
	}
}

func TestContentCompilerReturnsDiagnostics(t *testing.T) {
	compiler, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, diagnostics, err := compiler.Compile("Before {{ UNKNOWN }}")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if strings.TrimSpace(output) != "<p>Before </p>" {
		t.Fatalf("Compile() = %q", output)
	}

	if len(diagnostics) != 1 || string(diagnostics[0].Code) != "unknown-component" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestContentCompilerPreviewComponentReturnsDiagnostics(t *testing.T) {
	compiler, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, diagnostics, err := compiler.PreviewComponent("CARD", "# Title\n\n{{ UNKNOWN }}")
	if err != nil {
		t.Fatalf("PreviewComponent() error = %v", err)
	}

	if strings.TrimSpace(output) != "<h1>Title</h1>" {
		t.Fatalf("PreviewComponent() = %q", output)
	}

	if len(diagnostics) != 1 || string(diagnostics[0].Code) != "unknown-component" || !reflect.DeepEqual(diagnostics[0].Source, []string{"CARD"}) {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestContentCompilerBuildsComponoContext(t *testing.T) {
	compiler, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.CompileWithContext("Version: {{ context(app/version) }}", map[string]any{
		"app/version": "1.2.0",
	})
	if err != nil {
		t.Fatalf("CompileWithContext() error = %v", err)
	}

	if strings.TrimSpace(output) != "<p>Version: 1.2.0</p>" {
		t.Fatalf("CompileWithContext() = %q", output)
	}
}

func TestContentCompilerBuildsContextFromProviders(t *testing.T) {
	provider := &recordingContextProvider{
		values: map[string]any{
			"app/version": "1.2.0",
			"unused/key":  "unused",
		},
	}
	compiler, err := NewContentCompiler(nil, provider)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.CompileWithProviderContext(context.Background(), "Version: {{ context(app/version) }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}

	if strings.TrimSpace(output) != "<p>Version: 1.2.0</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}
	if !reflect.DeepEqual(provider.calls, [][]string{{"app/version"}}) {
		t.Fatalf("provider keys = %#v", provider.calls)
	}
}

func TestContentCompilerPassesEmptyKeysWhenSourceHasNoContextReferences(t *testing.T) {
	provider := &recordingContextProvider{
		values: map[string]any{
			"app/version": "1.2.0",
		},
	}
	compiler, err := NewContentCompiler(nil, provider)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.CompileWithProviderContext(context.Background(), "No context here.")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}

	if strings.TrimSpace(output) != "<p>No context here.</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}
	if !reflect.DeepEqual(provider.calls, [][]string{{}}) {
		t.Fatalf("provider keys = %#v", provider.calls)
	}
}

func TestContentCompilerBuildsProviderContextForGlobalComponents(t *testing.T) {
	provider := &recordingContextProvider{
		values: map[string]any{
			"user/name": "Jane",
		},
	}
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "GREETING", Content: "~ name = context(user/name)\nHello {{ name }}"},
	}, provider)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	output, _, err := compiler.CompileWithProviderContext(context.Background(), "{{ GREETING }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}

	if strings.TrimSpace(output) != "<p>Hello Jane</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}
	if !reflect.DeepEqual(provider.calls, [][]string{{"user/name"}}) {
		t.Fatalf("provider keys = %#v", provider.calls)
	}
}

func TestContentCompilerPreviewComponentAnalyzesRequiredParameterBody(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{{Name: "SITE_NAME", Content: "Umono"}})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	source := strings.Join([]string{
		"~ count! = 0",
		"{{ missing }}",
		"",
		"{{ UNKNOWN }} {{ SITE_NAME }} {{ GREET }}",
		"",
		"{{ MAIN content = NOPE }}",
		"",
		"~ GREET",
		"Hi",
	}, "\n")

	_, diagnostics, err := compiler.PreviewComponent("CARD", source)
	if err != nil {
		t.Fatalf("PreviewComponent() error = %v", err)
	}

	filtered := diagnostics[:0]
	for _, d := range diagnostics {
		if reflect.DeepEqual(d.Source, []string{"CARD"}) {
			filtered = append(filtered, d)
		}
	}

	type expected struct {
		code    string
		message string
		line    int
		column  int
	}
	want := []expected{
		{code: "unknown-parameter", line: 2, column: 1},
		{code: "unknown-component", message: "The component **UNKNOWN** is not defined.", line: 4, column: 1},
		{code: "unknown-component", message: "The component **NOPE** is not defined.", line: 6, column: 19},
	}

	if len(filtered) != len(want) {
		t.Fatalf("diagnostics = %#v", filtered)
	}

	for i, w := range want {
		got := filtered[i]
		if string(got.Code) != w.code {
			t.Fatalf("diagnostics = %#v", filtered)
		}
		if w.message != "" && got.Message != w.message {
			t.Fatalf("diagnostics = %#v", filtered)
		}
		if got.Range.Start.Line != w.line || got.Range.Start.Column != w.column {
			t.Fatalf("diagnostics = %#v", filtered)
		}
	}
}

func TestContentCompilerPreviewComponentDoesNotAnalyzeOptionalParameterBody(t *testing.T) {
	compiler, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	source := strings.Join([]string{
		"~ count = 0",
		"{{ missing }}",
		"",
		"~ UNUSED",
		"{{ Bad }}",
	}, "\n")

	_, diagnostics, err := compiler.PreviewComponent("CARD", source)
	if err != nil {
		t.Fatalf("PreviewComponent() error = %v", err)
	}

	filtered := diagnostics[:0]
	for _, d := range diagnostics {
		if reflect.DeepEqual(d.Source, []string{"CARD"}) {
			filtered = append(filtered, d)
		}
	}

	type expected struct {
		code    string
		message string
		line    int
		column  int
	}
	want := []expected{
		{code: "unknown-parameter", line: 2, column: 1},
	}

	if len(filtered) != len(want) {
		t.Fatalf("diagnostics = %#v", filtered)
	}

	for i, w := range want {
		got := filtered[i]
		if string(got.Code) != w.code {
			t.Fatalf("diagnostics = %#v", filtered)
		}
		if w.message != "" && got.Message != w.message {
			t.Fatalf("diagnostics = %#v", filtered)
		}
		if got.Range.Start.Line != w.line || got.Range.Start.Column != w.column {
			t.Fatalf("diagnostics = %#v", filtered)
		}
	}
}

type recordingContextProvider struct {
	values map[string]any
	calls  [][]string
}

func (p *recordingContextProvider) BuildCompileContext(_ context.Context, keys []string) (map[string]any, error) {
	if keys == nil {
		p.calls = append(p.calls, nil)

		values := map[string]any{}
		for key, value := range p.values {
			values[key] = value
		}
		return values, nil
	}

	copiedKeys := make([]string, len(keys))
	copy(copiedKeys, keys)
	p.calls = append(p.calls, copiedKeys)

	values := map[string]any{}
	for _, key := range keys {
		if value, ok := p.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func TestContentCompilerContextKeysUsedBy(t *testing.T) {
	compiler, err := NewContentCompiler([]models.Component{
		{Name: "OUTER", Content: "~ x = context(site/a)\n{{ x }} {{ INNER }}"},
		{Name: "INNER", Content: "{{ context(site/b) }}"},
		{Name: "UNUSED", Content: "{{ context(site/c) }}"},
		{Name: "LOOP_A", Content: "{{ LOOP_B }}"},
		{Name: "LOOP_B", Content: "{{ LOOP_A }} {{ context(site/loop) }}"},
	})
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "page and globals",
			source: "{{ OUTER }} {{ context(media-by-id/1) }}",
			want:   []string{"media-by-id/1", "site/a", "site/b"},
		},
		{
			name:   "no references",
			source: "Plain text",
			want:   nil,
		},
		{
			name:   "unused global",
			source: "{{ context(site/x) }}",
			want:   []string{"site/x"},
		},
		{
			name:   "cycle",
			source: "{{ LOOP_A }}",
			want:   []string{"site/loop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compiler.ContextKeysUsedBy(tt.source)

			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("ContextKeysUsedBy() = %#v, want no keys", got)
				}
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ContextKeysUsedBy() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
