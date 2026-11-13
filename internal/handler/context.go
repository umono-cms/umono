package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
)

// ContextHandler is the handler of the admin screen of the site contexts.
type ContextHandler struct {
	siteContextService *service.SiteContextService
	sitePageService    *service.SitePageService
}

// NewContextHandler returns the handler of the admin screen of the site
// contexts.
func NewContextHandler(s *service.SiteContextService, sps *service.SitePageService) *ContextHandler {
	return &ContextHandler{
		siteContextService: s,
		sitePageService:    sps,
	}
}

// Index sends the reader to the editor of a new site context.
func (h *ContextHandler) Index(c *fiber.Ctx) error {
	return c.Redirect("/admin/context/new")
}

// RenderNew renders the editor of a new site context.
func (h *ContextHandler) RenderNew(c *fiber.Ctx) error {
	return h.render(c, view.ContextEditor(models.SiteContext{}, "", "", c.Locals("I18n")), 0)
}

// RenderEdit renders the editor of the site context of the id.
func (h *ContextHandler) RenderEdit(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrNotFound
	}

	row, err := h.siteContextService.Get(uint(id))
	if err != nil {
		if errors.Is(err, service.ErrSiteContextNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}

	editor := view.ContextEditor(row, "", "", c.Locals("I18n"))
	return h.render(c, h.withPageCount(editor, row.Key), row.ID)
}

// Create creates the site context of the submitted key and source. A key or
// a source that is not valid is rendered in the editor of a new site context.
func (h *ContextHandler) Create(c *fiber.Ctx) error {
	key := strings.TrimSpace(c.FormValue("key"))
	source := c.FormValue("source")

	row, diagnostics, err := h.siteContextService.Create(key, source)
	if err != nil {
		keyErr, sourceErr, ok := contextErrorMessages(c, err)
		if !ok {
			return err
		}

		editor := view.ContextEditor(models.SiteContext{Key: key, Source: source}, keyErr, sourceErr, c.Locals("I18n"))
		if errors.Is(err, service.ErrInvalidSiteContextValue) {
			editor.Diagnostics = view.EditorDiagnostics(diagnostics, nil)
		}
		return h.render(c, editor, 0)
	}

	c.Set("HX-Push-Url", "/admin/context/"+strconv.FormatUint(uint64(row.ID), 10))
	editor := view.ContextEditor(row, "", "", c.Locals("I18n"))
	return h.render(c, h.withPageCount(editor, row.Key), row.ID)
}

// Update updates the key and the source of the site context of the id. A key
// or a source that is not valid is rendered in the editor of the site
// context.
func (h *ContextHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrNotFound
	}

	stored, err := h.siteContextService.Get(uint(id))
	if err != nil {
		if errors.Is(err, service.ErrSiteContextNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}

	// The key of the row in the database is the one the pages use until the
	// update is saved.
	storedKey := stored.Key

	key := strings.TrimSpace(c.FormValue("key"))
	source := c.FormValue("source")

	row, diagnostics, err := h.siteContextService.Update(uint(id), key, source)
	if err != nil {
		if errors.Is(err, service.ErrSiteContextNotFound) {
			return fiber.ErrNotFound
		}

		keyErr, sourceErr, ok := contextErrorMessages(c, err)
		if !ok {
			return err
		}

		// The site context keeps the last modified time of the stored row:
		// only its key and its source are the submitted ones.
		stored.Key = key
		stored.Source = source
		editor := view.ContextEditor(stored, keyErr, sourceErr, c.Locals("I18n"))
		if errors.Is(err, service.ErrInvalidSiteContextValue) {
			editor.Diagnostics = view.EditorDiagnostics(diagnostics, nil)
		}
		return h.render(c, h.withPageCount(editor, storedKey), stored.ID)
	}

	editor := view.ContextEditor(row, "", "", c.Locals("I18n"))
	return h.render(c, h.withPageCount(editor, row.Key), row.ID)
}

// Delete deletes the site context of the id.
func (h *ContextHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	row, err := h.siteContextService.Get(uint(id))
	if err != nil {
		if errors.Is(err, service.ErrSiteContextNotFound) {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return err
	}

	if err := h.siteContextService.Delete(row.Key); err != nil {
		if errors.Is(err, service.ErrSiteContextNotFound) {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return err
	}

	c.Set("HX-Redirect", "/admin/context/new")
	return c.SendStatus(fiber.StatusNoContent)
}

// Preview renders the diagnostics of the submitted source. An empty source
// has no diagnostics: clearing the source is not an error.
func (h *ContextHandler) Preview(c *fiber.Ctx) error {
	source := c.FormValue("source")

	diagnostics := []view.DiagnosticItem{}
	if strings.TrimSpace(source) != "" {
		diagnostics = view.EditorDiagnostics(h.siteContextService.Check(source), nil)
	}

	return Render(c, "partials/context-diagnostics", fiber.Map{
		"Diagnostics": diagnostics,
	})
}

// render renders the editor of a site context and the list of the site
// contexts.
func (h *ContextHandler) render(c *fiber.Ctx, data view.ContextEditorData, activeID uint) error {
	partial := "partials/context-editor"
	layouts := []string{"layouts/context", "layouts/admin"}

	if isContextHTMX(c) {
		partial = "partials/htmx/context-editor"
		layouts = []string{}
	}

	rows, err := h.siteContextService.List()
	if err != nil {
		return err
	}

	return Render(c, partial, fiber.Map{
		"ContextEditor": data,
		"ContextUl":     view.ContextUl(rows, activeID),
	}, layouts...)
}

// withPageCount returns the editor data with the number of pages that use
// the stored key of the site context. Only a saved site context has one.
func (h *ContextHandler) withPageCount(data view.ContextEditorData, storedKey string) view.ContextEditorData {
	if data.ID == 0 || h.sitePageService == nil {
		return data
	}

	data.PageCount = h.sitePageService.SiteContextPageCounts()[storedKey]
	return data
}

// contextErrorMessages returns the messages of the error of a key and of the
// error of a source. It reports whether the error is one of them.
func contextErrorMessages(c *fiber.Ctx, err error) (string, string, bool) {
	switch {
	case errors.Is(err, service.ErrInvalidSiteContextKey):
		return translatedValidationError(c, err), "", true
	case errors.Is(err, service.ErrSiteContextKeyAlreadyExists):
		return translatedValidationError(c, err), "", true
	case errors.Is(err, service.ErrInvalidSiteContextValue):
		return "", translatedValidationError(c, err), true
	default:
		return "", "", false
	}
}

// isContextHTMX reports whether the request of htmx swaps the content of the
// context screen. The requests that save a site context are only called by
// htmx and always swap the editor.
func isContextHTMX(c *fiber.Ctx) bool {
	if c.Get("HX-Request") != "true" {
		return false
	}

	switch c.Get("HX-Target") {
	case "context-content", "context-editor":
		return true
	}

	switch c.Method() {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodDelete:
		return true
	}

	return false
}
