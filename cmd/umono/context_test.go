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
	"github.com/umono-cms/umono/internal/models"
)

func sendForm(t *testing.T, app *fiber.App, method, path, cookie string, form url.Values) (*http.Response, string) {
	t.Helper()

	var requestBody io.Reader
	if form != nil {
		requestBody = strings.NewReader(form.Encode())
	}

	req := httptest.NewRequest(method, path, requestBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("HX-Request", "true")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: test: %v", method, path, err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatalf("%s %s: close body: %v", method, path, err)
	}

	return res, string(body)
}

func TestContextScreen(t *testing.T) {
	app, db := newTestApp(t)
	cookie := loginCookie(t, app)

	res, _ := sendForm(t, app, http.MethodPost, "/admin/context", cookie, url.Values{
		"key":    {"site/count"},
		"source": {"42"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create site context: status %d, want %d", res.StatusCode, http.StatusOK)
	}

	var row models.SiteContext
	if err := db.Where("key = ?", "site/count").First(&row).Error; err != nil {
		t.Fatalf("load site/count: %v", err)
	}
	if row.Source != "42" {
		t.Fatalf("site/count source = %q, want %q", row.Source, "42")
	}

	res, body := sendForm(t, app, http.MethodPost, "/admin/context", cookie, url.Values{
		"key":    {"bad key"},
		"source": {"1"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create invalid site context key: status %d, want %d", res.StatusCode, http.StatusOK)
	}
	var count int64
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 1 {
		t.Fatalf("site context count = %d, want 1", count)
	}
	if !strings.Contains(body, "site/") {
		t.Fatalf("invalid site context response body %q does not contain %q", body, "site/")
	}

	res, _ = sendForm(t, app, http.MethodPost, "/admin/context", cookie, url.Values{
		"key":    {"site/count"},
		"source": {"7"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create duplicate site context: status %d, want %d", res.StatusCode, http.StatusOK)
	}
	row = models.SiteContext{}
	if err := db.Where("key = ?", "site/count").First(&row).Error; err != nil {
		t.Fatalf("reload site/count: %v", err)
	}
	if row.Source != "42" {
		t.Fatalf("site/count source = %q, want %q", row.Source, "42")
	}
	count = 0
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 1 {
		t.Fatalf("site context count = %d, want 1", count)
	}

	res, _ = sendForm(t, app, http.MethodPost, "/admin/context", cookie, url.Values{
		"key":    {"site/other"},
		"source": {"[1,"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create invalid site context source: status %d, want %d", res.StatusCode, http.StatusOK)
	}
	count = 0
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 1 {
		t.Fatalf("site context count = %d, want 1", count)
	}

	id := row.ID
	path := "/admin/context/" + strconv.FormatUint(uint64(id), 10)
	res, _ = sendForm(t, app, http.MethodPut, path, cookie, url.Values{
		"key":    {"site/total"},
		"source": {"7"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update site context: status %d, want %d", res.StatusCode, http.StatusOK)
	}
	var updated models.SiteContext
	if err := db.Where("key = ?", "site/total").First(&updated).Error; err != nil {
		t.Fatalf("load site/total: %v", err)
	}
	if updated.ID != id {
		t.Fatalf("site/total ID = %d, want %d", updated.ID, id)
	}
	if updated.Source != "7" {
		t.Fatalf("site/total source = %q, want %q", updated.Source, "7")
	}

	res, _ = sendForm(t, app, http.MethodDelete, path, cookie, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete site context: status %d, want %d", res.StatusCode, http.StatusNoContent)
	}
	if redirect := res.Header.Get("HX-Redirect"); redirect != "/admin/context/new" {
		t.Fatalf("HX-Redirect = %q, want %q", redirect, "/admin/context/new")
	}
	count = 0
	if err := db.Model(&models.SiteContext{}).Count(&count).Error; err != nil {
		t.Fatalf("count site contexts: %v", err)
	}
	if count != 0 {
		t.Fatalf("site context count = %d, want 0", count)
	}

	res, _ = sendForm(t, app, http.MethodDelete, path, cookie, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing site context: status %d, want %d", res.StatusCode, http.StatusNotFound)
	}
}
