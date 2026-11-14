package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono"
	"github.com/umono-cms/umono/internal/i18n"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/theme"
	"github.com/umono-cms/umono/internal/view"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeThemeRegistry struct {
	err  error
	host string
}

func (f *fakeThemeRegistry) Verify(ctx context.Context, ref models.Contract, src []byte) error {
	return f.err
}

func (f *fakeThemeRegistry) Host() string {
	return f.host
}

func writeHandlerTheme(t *testing.T) string {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range []string{"built.toml", "contract.toml", "attributes.toml", "LICENSE"} {
		b, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		files[name] = b
	}
	files["assets/theme.css"] = themecore.FallbackCSS()
	sum, err := themecore.Checksums(themecore.Package{Files: files})
	if err != nil {
		t.Fatalf("themecore.Checksums() error = %v", err)
	}
	files["checksums.json"] = sum
	dir := t.TempDir()
	for name, b := range files {
		full := filepath.Join(dir, "acme", "minimal", "0.1.0", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
	}
	return dir
}

func newThemesTestAppWithDB(t *testing.T, reg service.ContractRegistry, middlewares ...fiber.Handler) (*fiber.App, *service.ThemeService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(&models.Contract{}, &models.SitePage{}, &models.Option{}); err != nil {
		t.Fatalf("db.AutoMigrate() error = %v", err)
	}

	ts := service.NewThemeService(
		writeHandlerTheme(t),
		t.TempDir(),
		service.NewContractService(repository.NewContractRepository(db)),
		repository.NewThemeRepository(db),
		reg,
	)
	h := NewSettingsHandler(service.NewSettingsService(), nil, nil, nil, ts)

	engine := html.NewFileSystem(http.FS(umono.Views()), ".html")
	engine.AddFunc("t", view.Translate)

	app := fiber.New(fiber.Config{Views: engine})
	for _, middleware := range middlewares {
		app.Use(middleware)
	}
	app.Get("/admin/settings/themes", h.RenderThemes)
	app.Get("/admin/settings/themes/pages", h.RenderThemePages)
	app.Post("/admin/settings/themes/activate", h.ActivateTheme)
	app.Post("/admin/settings/themes/deactivate", h.DeactivateTheme)

	return app, ts, db
}

func newThemesTestApp(t *testing.T, reg service.ContractRegistry) (*fiber.App, *service.ThemeService) {
	t.Helper()
	app, ts, _ := newThemesTestAppWithDB(t, reg)
	return app, ts
}

func loadThemesTestBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	bundle, err := i18n.LoadBundle(umono.Locales(), "en")
	if err != nil {
		t.Fatalf("i18n.LoadBundle() error = %v", err)
	}
	return bundle
}

func newThemesTestAppEnglish(t *testing.T, reg service.ContractRegistry) (*fiber.App, *service.ThemeService) {
	t.Helper()
	bundle := loadThemesTestBundle(t)
	app, ts, _ := newThemesTestAppWithDB(t, reg, func(c *fiber.Ctx) error {
		c.Locals("I18n", bundle.Translator("en"))
		return c.Next()
	})
	return app, ts
}

func htmxPost(t *testing.T, app *fiber.App, path string, form url.Values) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "settings-content")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return resp.StatusCode, string(body)
}

// htmxGet requests path as the settings screens do and returns the status
// and the body.
func htmxGet(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "settings-content")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return resp.StatusCode, string(body)
}

// seedUncoveredPages stores the records example/blog v1.0 and v1.1, a page
// bound to each and an unbound page. It returns the page bound to v1.1.
func seedUncoveredPages(t *testing.T, db *gorm.DB) models.SitePage {
	t.Helper()

	old := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0, Source: "old", Checksum: "old"}
	if result := db.Create(&old); result.Error != nil {
		t.Fatalf("db.Create() error = %v", result.Error)
	}
	newer := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 1, Source: "new", Checksum: "new"}
	if result := db.Create(&newer); result.Error != nil {
		t.Fatalf("db.Create() error = %v", result.Error)
	}

	if result := db.Create(&models.SitePage{Name: "Old Post", Slug: "old-post", ContractID: &old.ID}); result.Error != nil {
		t.Fatalf("db.Create() error = %v", result.Error)
	}
	newPage := models.SitePage{Name: "New Post", Slug: "new-post", ContractID: &newer.ID}
	if result := db.Create(&newPage); result.Error != nil {
		t.Fatalf("db.Create() error = %v", result.Error)
	}
	if result := db.Create(&models.SitePage{Name: "Free Page", Slug: "free-page"}); result.Error != nil {
		t.Fatalf("db.Create() error = %v", result.Error)
	}

	return newPage
}

func TestThemesPageListsTheGroups(t *testing.T) {
	app, _ := newThemesTestApp(t, &fakeThemeRegistry{})

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/themes", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "settings-content")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("resp.StatusCode = %d, want %d", got, want)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	bodyStr := string(body)

	for _, want := range []string{"example/blog v1", "acme/minimal", "0.1.0"} {
		if !strings.Contains(bodyStr, want) {
			t.Fatalf("body = %q, want to contain %q", bodyStr, want)
		}
	}
}

func TestThemesActivate(t *testing.T) {
	app, ts := newThemesTestApp(t, &fakeThemeRegistry{})

	status, body := htmxPost(t, app, "/admin/settings/themes/activate", url.Values{"path": {"acme/minimal/0.1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "settings.themes.activated") {
		t.Fatalf("body = %q, want to contain %q", body, "settings.themes.activated")
	}

	blog := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}.Group()
	if _, ok := ts.Loaded(blog); !ok {
		t.Fatalf("ts.Loaded(blog) = false, want true")
	}
}

func TestThemesActivateFailure(t *testing.T) {
	app, ts := newThemesTestApp(t, &fakeThemeRegistry{err: theme.ErrContractNotPublished})

	status, body := htmxPost(t, app, "/admin/settings/themes/activate", url.Values{"path": {"acme/minimal/0.1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	for _, want := range []string{"settings.themes.activate_failed", "not published"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want to contain %q", body, want)
		}
	}

	blog := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}.Group()
	if _, ok := ts.Loaded(blog); ok {
		t.Fatalf("ts.Loaded(blog) = true, want false")
	}
}

func TestThemesActivateOutdated(t *testing.T) {
	app, ts := newThemesTestApp(t, &fakeThemeRegistry{err: fmt.Errorf("contract.toml: layouts.post: 1:1: undefined component TABLE: %w", themecore.ErrOutdated)})

	status, body := htmxPost(t, app, "/admin/settings/themes/activate", url.Values{"path": {"acme/minimal/0.1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	for _, want := range []string{"settings.themes.activate_failed", "settings.themes.outdated"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want to contain %q", body, want)
		}
	}
	for _, unwanted := range []string{"undefined component TABLE", "themecore is older than the contract"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body = %q, want to not contain %q", body, unwanted)
		}
	}

	blog := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}.Group()
	if _, ok := ts.Loaded(blog); ok {
		t.Fatalf("ts.Loaded(blog) = true, want false")
	}
}

func TestThemesActivateContractSignature(t *testing.T) {
	app, ts := newThemesTestApp(t, &fakeThemeRegistry{err: fmt.Errorf("umono/page@v0.1: %w", fmt.Errorf("%w: line 2: signature does not match the contract", themecore.ErrContractSignature))})

	status, body := htmxPost(t, app, "/admin/settings/themes/activate", url.Values{"path": {"acme/minimal/0.1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	for _, want := range []string{"settings.themes.activate_failed", "settings.themes.contract_signature"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want to contain %q", body, want)
		}
	}
	for _, unwanted := range []string{"signature does not match the contract", "line 2"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body = %q, want to not contain %q", body, unwanted)
		}
	}

	blog := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}.Group()
	if _, ok := ts.Loaded(blog); ok {
		t.Fatalf("ts.Loaded(blog) = true, want false")
	}
}

func TestThemesDeactivate(t *testing.T) {
	app, ts := newThemesTestApp(t, &fakeThemeRegistry{})

	if _, err := ts.Activate(context.Background(), "acme/minimal/0.1.0"); err != nil {
		t.Fatalf("ts.Activate() error = %v", err)
	}

	status, body := htmxPost(t, app, "/admin/settings/themes/deactivate", url.Values{"contract": {"example/blog@v1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "settings.themes.deactivated") {
		t.Fatalf("body = %q, want to contain %q", body, "settings.themes.deactivated")
	}

	blog := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}.Group()
	if _, ok := ts.Loaded(blog); ok {
		t.Fatalf("ts.Loaded(blog) = true, want false")
	}
}

func TestThemesDeactivateRejectsABadContract(t *testing.T) {
	app, _ := newThemesTestApp(t, &fakeThemeRegistry{})

	status, _ := htmxPost(t, app, "/admin/settings/themes/deactivate", url.Values{"contract": {"nope"}})
	if got, want := status, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestThemePagesListsThePagesAboveTheMinor(t *testing.T) {
	app, _, db := newThemesTestAppWithDB(t, &fakeThemeRegistry{})
	page := seedUncoveredPages(t, db)

	status, body := htmxGet(t, app, "/admin/settings/themes/pages?contract=example/blog@v1.0")
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	for _, want := range []string{"New Post", "/new-post", "v1.1", fmt.Sprintf("/admin/site-pages/%d", page.ID)} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want to contain %q", body, want)
		}
	}
	for _, unwanted := range []string{"Old Post", "Free Page"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body = %q, want to not contain %q", body, unwanted)
		}
	}
}

func TestThemePagesRedirectsABadContract(t *testing.T) {
	app, _ := newThemesTestApp(t, &fakeThemeRegistry{})

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/themes/pages?contract=nope", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if got, want := resp.StatusCode, http.StatusSeeOther; got != want {
		t.Fatalf("resp.StatusCode = %d, want %d", got, want)
	}
	if got, want := resp.Header.Get("Location"), "/admin/settings/themes"; got != want {
		t.Fatalf("resp.Header.Get(\"Location\") = %q, want %q", got, want)
	}
}

func TestThemesPageLinksTheUncoveredPages(t *testing.T) {
	app, _, db := newThemesTestAppWithDB(t, &fakeThemeRegistry{})
	seedUncoveredPages(t, db)

	status, body := htmxGet(t, app, "/admin/settings/themes")
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "/admin/settings/themes/pages?contract=") {
		t.Fatalf("body = %q, want to contain %q", body, "/admin/settings/themes/pages?contract=")
	}
}

func TestThemesActivateConfirmNamesTheMirror(t *testing.T) {
	app, _ := newThemesTestAppEnglish(t, &fakeThemeRegistry{host: "registry.example.lan"})

	status, body := htmxGet(t, app, "/admin/settings/themes")
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "registry.example.lan") {
		t.Fatalf("body = %q, want to contain %q", body, "registry.example.lan")
	}
	for _, unwanted := range []string{"umono.dev", "{{host}}"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body = %q, want to not contain %q", body, unwanted)
		}
	}
}

func TestThemesActivatedNamesTheMirror(t *testing.T) {
	app, _ := newThemesTestAppEnglish(t, &fakeThemeRegistry{host: "registry.example.lan"})

	status, body := htmxPost(t, app, "/admin/settings/themes/activate", url.Values{"path": {"acme/minimal/0.1.0"}})
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "contract was verified on registry.example.lan.") {
		t.Fatalf("body = %q, want to contain %q", body, "contract was verified on registry.example.lan.")
	}
	if strings.Contains(body, "{{host}}") {
		t.Fatalf("body = %q, want to not contain %q", body, "{{host}}")
	}
}

func TestThemesConfirmNamesUmonoDev(t *testing.T) {
	app, _ := newThemesTestAppEnglish(t, &fakeThemeRegistry{host: "umono.dev"})

	status, body := htmxGet(t, app, "/admin/settings/themes")
	if got, want := status, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(body, "verified on umono.dev") {
		t.Fatalf("body = %q, want to contain %q", body, "verified on umono.dev")
	}
}

func TestLocalesNameTheRegistryHost(t *testing.T) {
	bundle := loadThemesTestBundle(t)
	keys := []string{
		"settings.themes.activate_confirm_registry",
		"settings.themes.activate_confirm_stored",
		"settings.themes.verified_registry",
		"settings.themes.verified_stored",
	}

	for _, locale := range bundle.SupportedLocales() {
		translator := bundle.Translator(locale.Code)
		for _, key := range keys {
			translation := translator.T(key)
			if !strings.Contains(translation, "{{host}}") {
				t.Errorf("locale %q translation %q = %q, want to contain %q", locale.Code, key, translation, "{{host}}")
			}
			if strings.Contains(translation, "umono.dev") {
				t.Errorf("locale %q translation %q = %q, want to not contain %q", locale.Code, key, translation, "umono.dev")
			}
		}
	}
}

func TestLocalesHaveTheContractSignatureMessage(t *testing.T) {
	bundle := loadThemesTestBundle(t)
	for _, locale := range bundle.SupportedLocales() {
		translation := bundle.Translator(locale.Code).T("settings.themes.contract_signature")
		if translation == "" {
			t.Errorf("locale %q translation %q = %q, want not empty", locale.Code, "settings.themes.contract_signature", translation)
		}
		if strings.HasPrefix(translation, i18n.MissingPrefix) {
			t.Errorf("locale %q translation %q = %q, want not to start with %q", locale.Code, "settings.themes.contract_signature", translation, i18n.MissingPrefix)
		}
	}
}
