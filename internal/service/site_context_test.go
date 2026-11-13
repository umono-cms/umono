package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSiteContextServiceParsesValuesAtStartup(t *testing.T) {
	db := newSiteContextTestDB(t)

	if err := db.Create(&models.SiteContext{Key: "site/count", Source: "42"}).Error; err != nil {
		t.Fatalf("create site count: %v", err)
	}
	if err := db.Create(&models.SiteContext{Key: "site/flags", Source: "[true, false]"}).Error; err != nil {
		t.Fatalf("create site flags: %v", err)
	}

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}

	want := map[string]any{
		"site/count": 42,
		"site/flags": []any{true, false},
	}
	got := svc.Values()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
}

func TestSiteContextServiceLeavesOutAnUnparseableValueAtStartup(t *testing.T) {
	db := newSiteContextTestDB(t)

	if err := db.Create(&models.SiteContext{Key: "site/count", Source: "42"}).Error; err != nil {
		t.Fatalf("create site count: %v", err)
	}
	if err := db.Create(&models.SiteContext{Key: "site/broken", Source: "[1,"}).Error; err != nil {
		t.Fatalf("create site broken: %v", err)
	}

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}

	want := map[string]any{
		"site/count": 42,
	}
	got := svc.Values()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
}

func TestSiteContextServiceSaveParsesTheValue(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}

	if _, err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.Save("site/count", "2"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	want := map[string]any{
		"site/count": 2,
	}
	got := svc.Values()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	reloaded, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	got = reloaded.Values()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	var count int64
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 1 {
		t.Fatalf("site context count = %d, want 1", count)
	}
}

func TestSiteContextServiceRejectsAnInvalidKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "no segment", key: "site"},
		{name: "trailing slash", key: "site/"},
		{name: "no namespace", key: "menu"},
		{name: "other namespace", key: "media-by-id/1"},
		{name: "empty segment", key: "site//menu"},
		{name: "trailing slash after segment", key: "site/menu/"},
		{name: "leading slash", key: "/site/menu"},
		{name: "uppercase letter", key: "site/Menu"},
		{name: "space", key: "site/me nu"},
		{name: "leading dash", key: "site/-menu"},
		{name: "trailing dash", key: "site/menu-"},
		{name: "two dashes", key: "site/foot--links"},
		{name: "underscore", key: "site/menu_x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newSiteContextTestDB(t)

			svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
			if err != nil {
				t.Fatalf("NewSiteContextService() error = %v", err)
			}

			if _, err := svc.Save(tc.key, "1"); !errors.Is(err, ErrInvalidSiteContextKey) {
				t.Fatalf("Save(%q) error = %v, want ErrInvalidSiteContextKey", tc.key, err)
			}

			var count int64
			if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
				t.Fatalf("count site contexts: %v", err)
			}
			if count != 0 {
				t.Fatalf("site context count = %d, want 0", count)
			}

			if values := svc.Values(); len(values) != 0 {
				t.Fatalf("Values() = %#v, want no values", values)
			}
		})
	}
}

func TestSiteContextServiceAcceptsAValidKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{name: "one segment", key: "site/menu"},
		{name: "dashed segment", key: "site/footer-links"},
		{name: "numeric segment", key: "site/1"},
		{name: "two segments", key: "site/nav/main-2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newSiteContextTestDB(t)

			svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
			if err != nil {
				t.Fatalf("NewSiteContextService() error = %v", err)
			}

			if _, err := svc.Save(tc.key, "1"); err != nil {
				t.Fatalf("Save(%q) error = %v, want nil", tc.key, err)
			}

			got, ok := svc.Values()[tc.key]
			if !ok {
				t.Fatalf("Values()[%q] is missing, want 1", tc.key)
			}
			if got != 1 {
				t.Fatalf("Values()[%q] = %#v, want 1", tc.key, got)
			}
		})
	}
}

func TestSiteContextServiceRejectsAnUnparseableValue(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "unterminated array", source: "[1,"},
		{name: "component", source: "FOO"},
		{name: "context expression", source: "context(site/name)"},
		{name: "parameter in record", source: "{ a: title }"},
		{name: "empty", source: ""},
		{name: "multiple values", source: "1 2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newSiteContextTestDB(t)

			svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
			if err != nil {
				t.Fatalf("NewSiteContextService() error = %v", err)
			}

			if _, err := svc.Save("site/count", "1"); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			diags, err := svc.Save("site/count", tc.source)
			if !errors.Is(err, ErrInvalidSiteContextValue) {
				t.Fatalf("Save(%q) error = %v, want ErrInvalidSiteContextValue", tc.source, err)
			}
			if len(diags) == 0 {
				t.Fatal("Save() diagnostics are empty, want diagnostics")
			}
			_, wantDiags := compono.ParseValue([]byte(tc.source))
			if !reflect.DeepEqual(diags, wantDiags) {
				t.Fatalf("Save() diagnostics = %#v, want %#v", diags, wantDiags)
			}

			wantValues := map[string]any{"site/count": 1}
			if got := svc.Values(); !reflect.DeepEqual(got, wantValues) {
				t.Fatalf("Values() = %#v, want %#v", got, wantValues)
			}

			var row models.SiteContext
			if err := db.Where("key = ?", "site/count").First(&row).Error; err != nil {
				t.Fatalf("get site count: %v", err)
			}
			if row.Source != "1" {
				t.Fatalf("Source = %q, want %q", row.Source, "1")
			}

			reloaded, err := NewSiteContextService(repository.NewSiteContextRepository(db))
			if err != nil {
				t.Fatalf("NewSiteContextService() error = %v", err)
			}
			if got := reloaded.Values(); !reflect.DeepEqual(got, wantValues) {
				t.Fatalf("Values() = %#v, want %#v", got, wantValues)
			}
		})
	}
}

func TestSiteContextServiceDoesNotCreateAnUnparseableValue(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}

	if _, err := svc.Save("site/count", "[1,"); !errors.Is(err, ErrInvalidSiteContextValue) {
		t.Fatalf("Save() error = %v, want ErrInvalidSiteContextValue", err)
	}

	var count int64
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 0 {
		t.Fatalf("site context count = %d, want 0", count)
	}

	if values := svc.Values(); len(values) != 0 {
		t.Fatalf("Values() = %#v, want no values", values)
	}
}

func newSiteContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SiteContext{}); err != nil {
		t.Fatalf("migrate site contexts: %v", err)
	}
	return db
}

func TestSiteContextServiceBuildsTheRequestedValues(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if _, err := svc.Save("site/count", "42"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.Save("site/flags", "[true]"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := svc.BuildCompileContext(context.Background(), []string{"site/count", "site/missing", "media-by-id/1"})
	if err != nil {
		t.Fatalf("BuildCompileContext() error = %v", err)
	}
	want := map[string]any{"site/count": 42}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildCompileContext() = %#v, want %#v", got, want)
	}
}

func TestSiteContextValuesReachPagesAndGlobals(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if _, err := svc.Save("site/count", "42"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	compiler, err := NewContentCompiler([]models.Component{
		{Name: "COUNTER", Content: "~ count = context(site/count)\nCount {{ count }}"},
	}, svc)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}
	ctx := context.Background()

	output, diagnostics, err := compiler.CompileWithProviderContext(ctx, "Total: {{ context(site/count) }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}
	if strings.TrimSpace(output) != "<p>Total: 42</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("CompileWithProviderContext() diagnostics = %#v, want none", diagnostics)
	}

	output, _, err = compiler.CompileWithProviderContext(ctx, "{{ COUNTER }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}
	if strings.TrimSpace(output) != "<p>Count 42</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}

	output, _, err = compiler.PreviewComponentWithProviderContext(ctx, "PREVIEWED", "Total: {{ context(site/count) }}")
	if err != nil {
		t.Fatalf("PreviewComponentWithProviderContext() error = %v", err)
	}
	if !strings.Contains(output, "Total: 42") {
		t.Fatalf("PreviewComponentWithProviderContext() = %q", output)
	}

	_, diagnostics, err = compiler.CompileWithProviderContext(ctx, "Total: {{ context(site/missing) }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}
	foundUnknownKey := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == compono.CodeUnknownKey {
			foundUnknownKey = true
			break
		}
	}
	if !foundUnknownKey {
		t.Fatalf("CompileWithProviderContext() diagnostics = %#v, want an unknown-key diagnostic", diagnostics)
	}

	if _, err := svc.Save("site/count", "7"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	output, _, err = compiler.CompileWithProviderContext(ctx, "Total: {{ context(site/count) }}")
	if err != nil {
		t.Fatalf("CompileWithProviderContext() error = %v", err)
	}
	if strings.TrimSpace(output) != "<p>Total: 7</p>" {
		t.Fatalf("CompileWithProviderContext() = %q", output)
	}
}

func TestSiteContextServiceDelete(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if _, err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.Save("site/other", "2"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := svc.Delete("site/count"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	want := map[string]any{"site/other": 2}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	reloaded, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if got := reloaded.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	var count int64
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 1 {
		t.Fatalf("site context count = %d, want 1", count)
	}

	if err := svc.Delete("site/count"); !errors.Is(err, ErrSiteContextNotFound) {
		t.Fatalf("Delete() error = %v, want ErrSiteContextNotFound", err)
	}
}

func TestSiteContextServiceRename(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if _, err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.Save("site/other", "2"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := svc.Rename("site/count", "site/total"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	want := map[string]any{"site/total": 1, "site/other": 2}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	reloaded, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if got := reloaded.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	if err := svc.Rename("site/total", "site/other"); !errors.Is(err, ErrSiteContextKeyAlreadyExists) {
		t.Fatalf("Rename() error = %v, want ErrSiteContextKeyAlreadyExists", err)
	}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	if err := svc.Rename("site/missing", "site/new"); !errors.Is(err, ErrSiteContextNotFound) {
		t.Fatalf("Rename() error = %v, want ErrSiteContextNotFound", err)
	}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	if err := svc.Rename("site/total", "total"); !errors.Is(err, ErrInvalidSiteContextKey) {
		t.Fatalf("Rename() error = %v, want ErrInvalidSiteContextKey", err)
	}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	if err := svc.Rename("site/total", "site/total"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if got := svc.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
}

func TestSiteContextDeletedOrRenamedKeyIsUnknown(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}
	if _, err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if _, err := svc.Save("site/other", "2"); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	compiler, err := NewContentCompiler(nil, svc)
	if err != nil {
		t.Fatalf("NewContentCompiler() error = %v", err)
	}

	hasUnknownKey := func(source string) bool {
		_, diagnostics, err := compiler.CompileWithProviderContext(context.Background(), source)
		if err != nil {
			t.Fatalf("CompileWithProviderContext() error = %v", err)
		}
		for _, diagnostic := range diagnostics {
			if diagnostic.Code == compono.CodeUnknownKey {
				return true
			}
		}
		return false
	}

	if hasUnknownKey("{{ context(site/count) }}") {
		t.Fatalf("hasUnknownKey(%q) = true, want false", "{{ context(site/count) }}")
	}

	if err := svc.Rename("site/count", "site/total"); err != nil {
		t.Fatalf("Rename() error = %v, want nil", err)
	}
	if !hasUnknownKey("{{ context(site/count) }}") {
		t.Fatalf("hasUnknownKey(%q) = false, want true", "{{ context(site/count) }}")
	}
	if hasUnknownKey("{{ context(site/total) }}") {
		t.Fatalf("hasUnknownKey(%q) = true, want false", "{{ context(site/total) }}")
	}

	if err := svc.Delete("site/other"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if !hasUnknownKey("{{ context(site/other) }}") {
		t.Fatalf("hasUnknownKey(%q) = false, want true", "{{ context(site/other) }}")
	}
}
