package handler

import (
	"errors"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/umono-cms/umono/internal/favicon"
	"github.com/umono-cms/umono/internal/service"
)

// faviconHrefLocal is the key of the local that holds the address the
// templates link the favicon with.
const faviconHrefLocal = "FaviconHref"

// FaviconSource gives the favicon that is served and whether there is one.
type FaviconSource interface {
	Current() (service.Favicon, bool)
}

// FaviconStore is a favicon that can be uploaded and removed. It is the set
// of methods of *service.FaviconService.
type FaviconStore interface {
	FaviconSource
	Upload(content []byte) error
	Remove() error
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
	current, ok := h.source.Current()
	if !ok {
		c.Status(fiber.StatusNotFound)
		return nil
	}

	etag := strconv.Quote(current.SHA256)

	c.Set("Content-Type", current.ContentType)
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-cache")
	c.Set("ETag", etag)

	if etagMatches(c.Get("If-None-Match"), etag) {
		c.Status(fiber.StatusNotModified)
		return nil
	}

	return c.Send(current.Content)
}

// faviconHref returns the address the templates link the favicon with:
// /favicon.ico?v= and the SHA-256 of the favicon, or "" when the site has
// none.
func faviconHref(source FaviconSource) string {
	current, ok := source.Current()
	if !ok {
		return ""
	}

	return "/favicon.ico?v=" + current.SHA256
}

// SetHref is a middleware that puts the address the templates link the favicon
// with into the locals: /favicon.ico?v= and the SHA-256 of the favicon, or ""
// when there is none. ?v= changes the address when the favicon changes, because
// browsers keep the favicon in a cache of their own. It reads the favicon on
// every request, so a new favicon is linked from the next request on.
func (h *FaviconHandler) SetHref(c *fiber.Ctx) error {
	c.Locals(faviconHrefLocal, faviconHref(h.source))
	return c.Next()
}

// FaviconSettingsHandler uploads and removes the favicon from Settings →
// General and answers with the favicon field.
type FaviconSettingsHandler struct {
	store FaviconStore
}

// NewFaviconSettingsHandler returns a handler that uploads and removes the
// favicon of the site.
func NewFaviconSettingsHandler(store FaviconStore) *FaviconSettingsHandler {
	return &FaviconSettingsHandler{store: store}
}

// Upload saves the favicon the owner uploads and answers with the favicon
// field of the settings. The file is read up to the size of the largest
// favicon and one byte more, so a bigger file is not read whole: the store
// rejects it.
func (h *FaviconSettingsHandler) Upload(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("favicon")
	if err != nil {
		return h.renderField(
			c,
			false,
			"settings.general.favicon_not_saved",
			"settings.general.favicon_file_required_description",
		)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, favicon.MaxSize+1))
	if err != nil {
		return err
	}

	if err := h.store.Upload(content); err != nil {
		switch {
		case errors.Is(err, favicon.ErrTooLarge):
			return h.renderField(
				c,
				false,
				"settings.general.favicon_not_saved",
				"settings.general.favicon_too_large_description",
			)
		case errors.Is(err, favicon.ErrFormat):
			return h.renderField(
				c,
				false,
				"settings.general.favicon_not_saved",
				"settings.general.favicon_invalid_format_description",
			)
		default:
			return err
		}
	}

	return h.renderField(
		c,
		true,
		"settings.general.favicon_saved",
		"settings.general.favicon_saved_description",
	)
}

// Remove removes the favicon of the site and answers with the favicon field
// of the settings.
func (h *FaviconSettingsHandler) Remove(c *fiber.Ctx) error {
	if err := h.store.Remove(); err != nil {
		return err
	}

	return h.renderField(
		c,
		true,
		"settings.general.favicon_removed",
		"settings.general.favicon_removed_description",
	)
}

// renderField answers with the favicon field of the settings. The address of
// the favicon is put into the locals again: the middleware that sets it ran
// before the change, so the field and the page show the favicon as it is now.
func (h *FaviconSettingsHandler) renderField(c *fiber.Ctx, success bool, titleKey, messageKey string) error {
	c.Locals(faviconHrefLocal, faviconHref(h.store))

	return Render(c, "partials/settings-favicon", fiber.Map{
		"FaviconStatusSuccess": success,
		"FaviconStatusTitle":   translate(c, titleKey),
		"FaviconStatusMessage": translate(c, messageKey),
	})
}
