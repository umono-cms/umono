package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/umono-cms/umono"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// dq is the double quote of the Compono source and of the HTML attributes.
const dq = "\""

// backslash is the backslash of the JavaScript string of the preview.
const backslash = "\\"

// newCurrentPreviewTestApp builds the app of the editor's preview. It holds
// no contract record and no 404 page: only the pages of the tests.
func newCurrentPreviewTestApp(t *testing.T) (*fiber.App, *service.SitePageService) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(
		&models.SitePage{},
		&models.Component{},
		&models.PageDiagnostic{},
		&models.Contract{},
		&models.Option{},
	); err != nil {
		t.Fatalf("db.AutoMigrate() error = %v", err)
	}

	cc, err := service.NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("service.NewContentCompiler() error = %v", err)
	}

	optionRepo := repository.NewOptionRepository(db)
	sps := service.NewSitePageService(repository.NewSitePageRepository(db), optionRepo, cc)
	sps.SetPageDiagnosticRepository(repository.NewPageDiagnosticRepository(db))

	cs := service.NewComponentService(repository.NewComponentRepository(db))
	cs.SetContentCompiler(cc)

	ts := service.NewThemeService(
		writeHandlerTheme(t),
		t.TempDir(),
		service.NewContractService(repository.NewContractRepository(db)),
		repository.NewThemeRepository(db),
		&fakeThemeRegistry{},
	)
	sps.SetPageThemes(ts)

	engine := html.NewFileSystem(http.FS(umono.Views()), ".html")
	engine.AddFunc("t", view.Translate)

	app := fiber.New(fiber.Config{Views: engine})

	ph := NewPreviewHandler(sps, cs)
	app.Post("/admin/site-pages/preview", ph.RenderSitePagePreview)
	app.Post("/admin/components/preview", ph.RenderComponentPreview)

	sph := NewSitePageHandler(sps, cs, ts)
	app.Post("/admin/site-pages", sph.Create)
	app.Put("/admin/site-pages", sph.Update)

	return app, sps
}

// autoLink returns the Compono source of a LINK whose current is resolved
// against the address of the page that renders it.
func autoLink(url string) string {
	return fmt.Sprintf("{{ LINK text = %sx%s url = %s%s%s current = %s__auto%s }}%s",
		dq, dq, dq, url, dq, dq, dq, "\n\n")
}

// markedLink reports whether the body has the link of the url marked as the
// address of the page that renders it. It also finds the link in the preview
// the page editor embeds as a JavaScript string.
func markedLink(body, url string) bool {
	unescape := strings.NewReplacer(backslash+"u003c", "<", backslash+"u003e", ">", backslash+dq, dq)
	return strings.Contains(unescape.Replace(body), fmt.Sprintf("<a href=%s%s%s aria-current=%spage%s>",
		dq, url, dq, dq, dq))
}

// formRequest sends the form to the app with the method and returns the body
// of the response.
func formRequest(t *testing.T, app *fiber.App, method, path string, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(body)
}

func TestPreviewMarksTheSavedSlug(t *testing.T) {
	app, sps := newCurrentPreviewTestApp(t)

	page, errs := sps.Create(models.SitePage{Name: "About", Slug: "about", Enabled: true})
	if len(errs) > 0 {
		t.Fatalf("sps.Create() errors = %v", errs)
	}

	body := formRequest(t, app, http.MethodPost, "/admin/site-pages/preview", url.Values{
		"id":      {fmt.Sprintf("%d", page.ID)},
		"slug":    {"renamed"},
		"content": {autoLink("/about") + autoLink("/renamed")},
	})

	if !markedLink(body, "/about") {
		t.Errorf("body does not mark %q; body = %q", "/about", body)
	}
	if markedLink(body, "/renamed") {
		t.Errorf("body marks %q; body = %q", "/renamed", body)
	}
}

func TestPreviewOfANewPageMarksNothing(t *testing.T) {
	app, _ := newCurrentPreviewTestApp(t)

	body := formRequest(t, app, http.MethodPost, "/admin/site-pages/preview", url.Values{
		"id":      {"0"},
		"slug":    {"about"},
		"content": {autoLink("/about") + autoLink("/")},
	})

	if strings.Contains(body, "aria-current") {
		t.Errorf("body contains %q; body = %q", "aria-current", body)
	}
}

func TestPreviewOfTheGlobalEditorMarksNothing(t *testing.T) {
	app, _ := newCurrentPreviewTestApp(t)

	body := formRequest(t, app, http.MethodPost, "/admin/components/preview", url.Values{
		"name":    {"MENU"},
		"content": {autoLink("/") + autoLink("/about")},
	})

	if strings.Contains(body, "aria-current") {
		t.Errorf("body contains %q; body = %q", "aria-current", body)
	}
}

func TestUpdateFailurePreviewMarksTheSavedSlug(t *testing.T) {
	app, sps := newCurrentPreviewTestApp(t)

	page, errs := sps.Create(models.SitePage{Name: "About", Slug: "about", Enabled: true})
	if len(errs) > 0 {
		t.Fatalf("sps.Create() errors = %v", errs)
	}

	body := formRequest(t, app, http.MethodPut, "/admin/site-pages", url.Values{
		"id":      {fmt.Sprintf("%d", page.ID)},
		"name":    {"About"},
		"slug":    {"Bad Slug"},
		"enabled": {"1"},
		"content": {autoLink("/about") + autoLink("/Bad Slug")},
	})

	if !markedLink(body, "/about") {
		t.Errorf("body does not mark %q; body = %q", "/about", body)
	}
}

func TestUpdatePreviewMarksTheNewSlug(t *testing.T) {
	app, sps := newCurrentPreviewTestApp(t)

	page, errs := sps.Create(models.SitePage{Name: "About", Slug: "about", Enabled: true})
	if len(errs) > 0 {
		t.Fatalf("sps.Create() errors = %v", errs)
	}

	body := formRequest(t, app, http.MethodPut, "/admin/site-pages", url.Values{
		"id":      {fmt.Sprintf("%d", page.ID)},
		"name":    {"About"},
		"slug":    {"renamed"},
		"enabled": {"1"},
		"content": {autoLink("/about") + autoLink("/renamed")},
	})

	if !markedLink(body, "/renamed") {
		t.Errorf("body does not mark %q; body = %q", "/renamed", body)
	}
	if markedLink(body, "/about") {
		t.Errorf("body marks %q; body = %q", "/about", body)
	}
}

func TestCreatePreviewMarksTheCreatedSlug(t *testing.T) {
	app, _ := newCurrentPreviewTestApp(t)

	body := formRequest(t, app, http.MethodPost, "/admin/site-pages", url.Values{
		"name":    {"New"},
		"slug":    {"new"},
		"enabled": {"1"},
		"content": {autoLink("/new")},
	})

	if !markedLink(body, "/new") {
		t.Errorf("body does not mark %q; body = %q", "/new", body)
	}
}

func TestCreateFailurePreviewMarksNothing(t *testing.T) {
	app, _ := newCurrentPreviewTestApp(t)

	body := formRequest(t, app, http.MethodPost, "/admin/site-pages", url.Values{
		"name":    {""},
		"slug":    {"new"},
		"content": {autoLink("/new")},
	})

	if strings.Contains(body, "aria-current") {
		t.Errorf("body contains %q; body = %q", "aria-current", body)
	}
}
