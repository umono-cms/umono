package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/umono-cms/umono/internal/service"
)

// faviconHrefLocal is the key of the local that holds the address the
// templates link the favicon with.
const faviconHrefLocal = "FaviconHref"

// FaviconSource gives the favicon that is served and whether there is one.
type FaviconSource interface {
	Current() (service.Favicon, bool)
}

// FaviconHandler serves the favicon of the site at /favicon.ico from memory.
// It has the cache of the shell files: no-cache and an ETag from the SHA-256
// of the content. The query string, such as the ?v= the templates add, is not
// looked at.
type FaviconHandler struct {
	source FaviconSource
}

// NewFaviconHandler returns a handler that serves the favicon of source.
func NewFaviconHandler(source FaviconSource) *FaviconHandler {
	return &FaviconHandler{source: source}
}

// Serve writes the favicon of the site, or answers 404 when the site has none.
// The 404 page of the site is not rendered: the body stays empty.
func (h *FaviconHandler) Serve(c *fiber.Ctx) error {
	favicon, ok := h.source.Current()
	if !ok {
		c.Status(fiber.StatusNotFound)
		return nil
	}

	etag := strconv.Quote(favicon.SHA256)

	c.Set("Content-Type", favicon.ContentType)
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-cache")
	c.Set("ETag", etag)

	if etagMatches(c.Get("If-None-Match"), etag) {
		c.Status(fiber.StatusNotModified)
		return nil
	}

	return c.Send(favicon.Content)
}

// SetHref is a middleware that puts the address the templates link the favicon
// with into the locals: /favicon.ico?v= and the SHA-256 of the favicon, or ""
// when there is none. ?v= changes the address when the favicon changes, because
// browsers keep the favicon in a cache of their own. It reads the favicon on
// every request, so a new favicon is linked from the next request on.
func (h *FaviconHandler) SetHref(c *fiber.Ctx) error {
	href := ""
	if favicon, ok := h.source.Current(); ok {
		href = "/favicon.ico?v=" + favicon.SHA256
	}
	c.Locals(faviconHrefLocal, href)
	return c.Next()
}
