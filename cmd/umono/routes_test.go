package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	umonocrypto "github.com/umono-cms/crypto"
	"github.com/umono-cms/umono/internal/credentials"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/theme"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	testAdminUsername = "admin"
	testAdminPassword = "password"
	testUmonoSecret   = "2b7e151628aed2a6abf7158809cf4f3c762e7160f38b4da56a784d9045190cfe"
)

// newTestApp builds the app in a temporary working directory with an empty
// database and the admin credentials testAdminUsername and testAdminPassword.
func newTestApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Chdir(t.TempDir())

	for _, kv := range []struct{ key, value string }{
		{credentials.HashedUsernameEnvKey, testAdminUsername},
		{credentials.HashedPasswordEnvKey, testAdminPassword},
	} {
		h, err := credentials.Hash(kv.value)
		if err != nil {
			t.Fatalf("hash %s: %v", kv.key, err)
		}
		t.Setenv(kv.key, credentials.Encode(h))
	}

	t.Setenv("SESSION_DRIVER", "")

	db, err := gorm.Open(sqlite.Open("umono.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	key, err := umonocrypto.ParseHexString(testUmonoSecret)
	if err != nil {
		t.Fatalf("parse hex: %v", err)
	}

	secret, err := umonocrypto.New(key, []byte("umono-secrets"))
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}

	app, err := newApp(db, secret, "", theme.RegistryURL)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	return app, db
}

// loginCookie signs in and returns the session cookie as a Cookie header value.
func loginCookie(t *testing.T, app *fiber.App) string {
	form := url.Values{"username": {testAdminUsername}, "password": {testAdminPassword}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}

	for _, c := range res.Cookies() {
		if c.Name == "session_id" {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatalf("login did not set a session cookie, status %d", res.StatusCode)
	return ""
}

// seedContent stores a page, a global, a media and a site context in db and
// returns the value of the :id parameter of each admin route prefix.
func seedContent(t *testing.T, db *gorm.DB) map[string]string {
	page := models.SitePage{Name: "Home", Slug: "home", Content: "Hello {{ GREETING }}", Enabled: true}
	if result := db.Create(&page); result.Error != nil {
		t.Fatalf("create page: %v", result.Error)
	}

	component := models.Component{Name: "GREETING", Content: "Hi"}
	if result := db.Create(&component); result.Error != nil {
		t.Fatalf("create component: %v", result.Error)
	}

	var storage models.Storage
	if result := db.First(&storage); result.Error != nil {
		t.Fatalf("load default storage: %v", result.Error)
	}

	media := models.Media{ID: "seed-media", StorageID: storage.ID, OriginalName: "seed.png", PathKey: "seed.png", MimeType: "image/png", Size: 1, Hash: "seed"}
	if result := db.Create(&media); result.Error != nil {
		t.Fatalf("create media: %v", result.Error)
	}

	siteContext := models.SiteContext{Key: "site/count", Source: "42"}
	if result := db.Create(&siteContext); result.Error != nil {
		t.Fatalf("create site context: %v", result.Error)
	}

	return map[string]string{
		"/admin/site-pages/":       strconv.FormatUint(uint64(page.ID), 10),
		"/admin/components/":       strconv.FormatUint(uint64(component.ID), 10),
		"/admin/media/":            media.ID,
		"/admin/settings/storage/": storage.ID,
		"/admin/context/":          strconv.FormatUint(uint64(siteContext.ID), 10),
	}
}

// skippedRoutes are the admin GET routes the test does not render, with the
// reason.
var skippedRoutes = map[string]string{
	"/admin/login":                        "rendered signed out at the end of the test",
	"/admin/media/pending/:token/preview": "needs a pending upload",
}

// get requests path and follows the redirects. It returns the last status and
// body.
func get(t *testing.T, app *fiber.App, path, cookie string, htmx bool) (int, string) {
	t.Helper()
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s: test: %v", path, err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatalf("%s: read body: %v", path, err)
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 && res.Header.Get("Location") != "" {
			path = res.Header.Get("Location")
			continue
		}
		return res.StatusCode, string(body)
	}
	t.Fatalf("%s: too many redirects", path)
	return 0, ""
}

// routePath fills the :id parameter of an admin route from ids, which maps a
// route prefix to the value. It reports false if the route has another
// parameter or no value.
func routePath(route string, ids map[string]string) (string, bool) {
	i := strings.Index(route, ":")
	if i < 0 {
		return route, true
	}
	if !strings.HasPrefix(route[i:], ":id") {
		return "", false
	}
	id, ok := ids[route[:i]]
	if !ok {
		return "", false
	}
	path := route[:i] + id + route[i+len(":id"):]
	if strings.Contains(path, ":") {
		return "", false
	}
	return path, true
}

// checkRender reports a status that is not 2xx and a template error in body.
func checkRender(t *testing.T, name string, status int, body string) {
	t.Helper()
	if status < 200 || status > 299 {
		t.Errorf("%s: status %d", name, status)
	}
	if i := strings.Index(body, "template:"); i >= 0 {
		t.Errorf("%s: template error: %s", name, body[i:min(len(body), i+200)])
	}
}

func TestAdminRoutesRender(t *testing.T) {
	app, db := newTestApp(t)
	ids := seedContent(t, db)
	cookie := loginCookie(t, app)

	for _, route := range app.GetRoutes(true) {
		if route.Method != http.MethodGet || !strings.HasPrefix(route.Path, "/admin") {
			continue
		}
		if reason, ok := skippedRoutes[route.Path]; ok {
			t.Logf("skip %s: %s", route.Path, reason)
			continue
		}
		path, ok := routePath(route.Path, ids)
		if !ok {
			t.Errorf("%s: no value for its parameters", route.Path)
			continue
		}
		status, body := get(t, app, path, cookie, true)
		checkRender(t, path+" (htmx)", status, body)
		status, body = get(t, app, path, cookie, false)
		// OnlyHTMX answers a full page request with 404.
		if status == http.StatusNotFound {
			continue
		}
		checkRender(t, path+" (full page)", status, body)
	}

	status, body := get(t, app, "/admin/login", "", false)
	checkRender(t, "/admin/login", status, body)
}
