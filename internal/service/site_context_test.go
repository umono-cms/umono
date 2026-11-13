package service

import (
	"errors"
	"reflect"
	"testing"

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

	if err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := svc.Save("site/count", "2"); err != nil {
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

			if err := svc.Save(tc.key, "1"); !errors.Is(err, ErrInvalidSiteContextKey) {
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

			if err := svc.Save(tc.key, "1"); err != nil {
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
