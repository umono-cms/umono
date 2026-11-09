package service

import (
	"strings"
	"testing"

	"github.com/umono-cms/umono"
	"github.com/umono-cms/umono/internal/i18n"
)

func TestGlobalPlaceholderCompilesInEveryLocale(t *testing.T) {
	bundle, err := i18n.LoadBundle(umono.Locales(), DefaultLanguage)
	if err != nil {
		t.Fatalf("LoadBundle() error = %v", err)
	}

	for _, locale := range bundle.SupportedLocales() {
		t.Run(locale.Code, func(t *testing.T) {
			placeholder := bundle.Translator(locale.Code).T("admin.components.placeholders.content")
			if !strings.HasPrefix(placeholder, "~ ") {
				t.Fatalf("placeholder = %q, want prefix %q", placeholder, "~ ")
			}

			compiler, err := NewContentCompiler(nil)
			if err != nil {
				t.Fatalf("NewContentCompiler() error = %v", err)
			}

			_, diagnostics, err := compiler.PreviewComponent("GREETING", placeholder)
			if err != nil {
				t.Fatalf("PreviewComponent() error = %v", err)
			}

			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
		})
	}
}
