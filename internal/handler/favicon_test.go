package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/umono-cms/umono/internal/service"
)

type stubFaviconSource struct {
	favicon service.Favicon
	ok      bool
}

func (s stubFaviconSource) Current() (service.Favicon, bool) {
	return s.favicon, s.ok
}

func newFaviconTestApp(t *testing.T, source FaviconSource) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Get("/favicon.ico", NewFaviconHandler(source).Serve)
	return app
}

func TestFaviconServesTheFavicon(t *testing.T) {
	cases := []struct {
		name    string
		favicon service.Favicon
	}{
		{
			name: "png",
			favicon: service.Favicon{
				Content:     []byte("\x89PNG\r\n\x1a\npng"),
				ContentType: "image/png",
				SHA256:      "aaaa",
			},
		},
		{
			name: "ico",
			favicon: service.Favicon{
				Content:     []byte("\x00\x00\x01\x00ico"),
				ContentType: "image/x-icon",
				SHA256:      "bbbb",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newFaviconTestApp(t, stubFaviconSource{favicon: tc.favicon, ok: true})

			req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
			}

			if got := resp.Header.Get("Content-Type"); got != tc.favicon.ContentType {
				t.Fatalf("got Content-Type %q, want %q", got, tc.favicon.ContentType)
			}

			if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("got X-Content-Type-Options %q, want %q", got, "nosniff")
			}

			if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("got Cache-Control %q, want %q", got, "no-cache")
			}

			wantETag := strconv.Quote(tc.favicon.SHA256)
			if got := resp.Header.Get("ETag"); got != wantETag {
				t.Fatalf("got ETag %q, want %q", got, wantETag)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !bytes.Equal(body, tc.favicon.Content) {
				t.Fatalf("got body %q, want %q", string(body), string(tc.favicon.Content))
			}
		})
	}
}

func TestFaviconIgnoresTheVersionQuery(t *testing.T) {
	favicon := service.Favicon{
		Content:     []byte("\x89PNG\r\n\x1a\npng"),
		ContentType: "image/png",
		SHA256:      "aaaa",
	}
	app := newFaviconTestApp(t, stubFaviconSource{favicon: favicon, ok: true})

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico?v=anything", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(body, favicon.Content) {
		t.Fatalf("got body %q, want %q", string(body), string(favicon.Content))
	}
}

func TestFaviconAnswersAMatchingETagWithNotModified(t *testing.T) {
	favicon := service.Favicon{
		Content:     []byte("\x89PNG\r\n\x1a\npng"),
		ContentType: "image/png",
		SHA256:      "aaaa",
	}
	app := newFaviconTestApp(t, stubFaviconSource{favicon: favicon, ok: true})

	cases := []struct {
		name        string
		ifNoneMatch string
		wantStatus  int
		wantBody    []byte
	}{
		{
			name:        "matching tag",
			ifNoneMatch: strconv.Quote("aaaa"),
			wantStatus:  http.StatusNotModified,
			wantBody:    []byte{},
		},
		{
			name:        "other tag",
			ifNoneMatch: strconv.Quote("other"),
			wantStatus:  http.StatusOK,
			wantBody:    favicon.Content,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
			req.Header.Set("If-None-Match", tc.ifNoneMatch)

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got status %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !bytes.Equal(body, tc.wantBody) {
				t.Fatalf("got body %q, want %q", string(body), string(tc.wantBody))
			}
		})
	}
}

func TestFaviconWithoutAFaviconIsNotFound(t *testing.T) {
	app := newFaviconTestApp(t, stubFaviconSource{})

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("got body %q, want empty", string(body))
	}
}
