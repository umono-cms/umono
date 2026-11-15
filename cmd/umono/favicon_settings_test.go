package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// faviconTestPNG returns a valid one-pixel PNG image.
func faviconTestPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}

	return buf.Bytes()
}

// faviconTestICO returns an ICO containing png as its only image.
func faviconTestICO(t *testing.T, png []byte) []byte {
	t.Helper()

	ico := make([]byte, 22+len(png))
	binary.LittleEndian.PutUint16(ico[0:2], 0)
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)

	ico[6] = 1
	ico[7] = 1
	ico[8] = 0
	ico[9] = 0
	binary.LittleEndian.PutUint16(ico[10:12], 1)
	binary.LittleEndian.PutUint16(ico[12:14], 32)
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(png)))
	binary.LittleEndian.PutUint32(ico[18:22], 22)

	copy(ico[22:], png)
	return ico
}

// faviconHash returns the lowercase hexadecimal SHA-256 of content.
func faviconHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// sendFavicon sends a favicon form request to the admin settings endpoint.
func sendFavicon(t *testing.T, app *fiber.App, cookie, method, name string, content []byte) (int, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if content != nil {
		part, err := writer.CreateFormFile("favicon", name)
		if err != nil {
			t.Fatalf("create favicon form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write favicon form file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart form: %v", err)
	}

	req := httptest.NewRequest(method, "/admin/settings/favicon", &body)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("HX-Request", "true")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, string(responseBody)
}

// getFavicon requests the favicon and returns its response.
func getFavicon(t *testing.T, app *fiber.App) (int, string, []byte) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), body
}

func TestFaviconSettingsUploadsAPNG(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.png", png)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon saved") {
		t.Fatalf("response body does not contain %q: %s", "Favicon saved", body)
	}

	src := `src="/favicon.ico?v=` + faviconHash(png) + `"`
	if !strings.Contains(body, src) {
		t.Fatalf("response body does not contain %q: %s", src, body)
	}

	status, contentType, content := getFavicon(t, app)
	if status != http.StatusOK {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusOK)
	}
	if contentType != "image/png" {
		t.Fatalf("got favicon Content-Type %q, want %q", contentType, "image/png")
	}
	if !bytes.Equal(content, png) {
		t.Fatalf("got favicon body %q, want %q", content, png)
	}

	page := getPage(t, app, "/admin/settings/general", cookie)
	if !strings.Contains(page, src) {
		t.Fatalf("settings page does not contain %q", src)
	}
}

func TestFaviconSettingsUploadsAnICO(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)
	ico := faviconTestICO(t, png)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.ico", ico)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon saved") {
		t.Fatalf("response body does not contain %q: %s", "Favicon saved", body)
	}

	status, contentType, content := getFavicon(t, app)
	if status != http.StatusOK {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusOK)
	}
	if contentType != "image/x-icon" {
		t.Fatalf("got favicon Content-Type %q, want %q", contentType, "image/x-icon")
	}
	if !bytes.Equal(content, ico) {
		t.Fatalf("got favicon body %q, want %q", content, ico)
	}
}

// TestFaviconSettingsRejectsAFileThatIsNotPNGOrICO checks that a file which is neither a PNG nor an ICO image is not saved.
func TestFaviconSettingsRejectsAFileThatIsNotPNGOrICO(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.png", png)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}

	status, body = sendFavicon(t, app, cookie, http.MethodPost, "icon.png", []byte("GIF89a\x01\x00\x01\x00"))
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon was not saved") {
		t.Fatalf("response body does not contain %q: %s", "Favicon was not saved", body)
	}
	if !strings.Contains(body, "is not a PNG or ICO image") {
		t.Fatalf("response body does not contain %q: %s", "is not a PNG or ICO image", body)
	}

	status, _, content := getFavicon(t, app)
	if status != http.StatusOK {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusOK)
	}
	if !bytes.Equal(content, png) {
		t.Fatalf("got favicon body %q, want %q", content, png)
	}
}

// TestFaviconSettingsRejectsAFileLargerThan256KiB checks that a file bigger than 256 KiB is not saved.
func TestFaviconSettingsRejectsAFileLargerThan256KiB(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.png", png)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}

	status, body = sendFavicon(t, app, cookie, http.MethodPost, "icon.png", append(bytes.Clone(png), make([]byte, 256<<10)...))
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon was not saved") {
		t.Fatalf("response body does not contain %q: %s", "Favicon was not saved", body)
	}
	if !strings.Contains(body, "larger than 256 KiB") {
		t.Fatalf("response body does not contain %q: %s", "larger than 256 KiB", body)
	}

	status, _, content := getFavicon(t, app)
	if status != http.StatusOK {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusOK)
	}
	if !bytes.Equal(content, png) {
		t.Fatalf("got favicon body %q, want %q", content, png)
	}
}

// TestFaviconSettingsWithoutAFile checks that a request without a file does not save a favicon.
func TestFaviconSettingsWithoutAFile(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.png", png)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}

	status, body = sendFavicon(t, app, cookie, http.MethodPost, "icon.png", nil)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon was not saved") {
		t.Fatalf("response body does not contain %q: %s", "Favicon was not saved", body)
	}
	if !strings.Contains(body, "Choose a PNG or ICO file") {
		t.Fatalf("response body does not contain %q: %s", "Choose a PNG or ICO file", body)
	}

	status, _, content := getFavicon(t, app)
	if status != http.StatusOK {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusOK)
	}
	if !bytes.Equal(content, png) {
		t.Fatalf("got favicon body %q, want %q", content, png)
	}
}

// TestFaviconSettingsRemovesTheFavicon checks that the favicon is removed and is no longer served.
func TestFaviconSettingsRemovesTheFavicon(t *testing.T) {
	app, _ := newTestApp(t)
	cookie := loginCookie(t, app)
	png := faviconTestPNG(t)

	status, body := sendFavicon(t, app, cookie, http.MethodPost, "icon.png", png)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	src := `src="/favicon.ico?v=` + faviconHash(png) + `"`
	if !strings.Contains(body, src) {
		t.Fatalf("response body does not contain %q: %s", src, body)
	}

	status, body = sendFavicon(t, app, cookie, http.MethodDelete, "", nil)
	if status != http.StatusOK {
		t.Fatalf("got status %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(body, "Favicon removed") {
		t.Fatalf("response body does not contain %q: %s", "Favicon removed", body)
	}
	if strings.Contains(body, "/favicon.ico?v=") {
		t.Fatalf("response body contains %q: %s", "/favicon.ico?v=", body)
	}

	status, _, _ = getFavicon(t, app)
	if status != http.StatusNotFound {
		t.Fatalf("got favicon status %d, want %d", status, http.StatusNotFound)
	}
}
