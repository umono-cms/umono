package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/internal/service"
)

type previewHandler struct {
	sitePageService  *service.SitePageService
	componentService *service.ComponentService
}

func NewPreviewHandler(sps *service.SitePageService, cs *service.ComponentService) *previewHandler {
	return &previewHandler{
		sitePageService:  sps,
		componentService: cs,
	}
}

// TODO: Change names
// RenderSitePagePreview -> SitePageReview
// RenderComponentPreview -> ComponentPreview

func (h *previewHandler) RenderSitePagePreview(c *fiber.Ctx) error {
	var req struct {
		Content string `form:"content"`
		Layout  string `form:"layout"`
	}

	if err := c.BodyParser(&req); err != nil {
		return err
	}

	contractID, layoutID, err := bindingValue(req.Layout)
	if err != nil {
		return err
	}

	pageID := uint64(0)
	if n, err := strconv.ParseUint(c.FormValue("id"), 10, 0); err == nil {
		pageID = n
	}

	output, diagnostics := sitePagePreviewBound(c.UserContext(), h.sitePageService, uint(pageID), req.Content, contractID, layoutID)

	return Render(c, "partials/editor-preview", fiber.Map{
		"Output":      output,
		"Diagnostics": diagnostics,
	})
}

func (h *previewHandler) RenderComponentPreview(c *fiber.Ctx) error {
	var req struct {
		Content string `form:"content"`
		Name    string `form:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return err
	}

	output, diagnostics := componentPreview(c.UserContext(), h.componentService, req.Name, req.Content)

	return Render(c, "partials/editor-preview", fiber.Map{
		"Output":      output,
		"Diagnostics": diagnostics,
	})
}

func (h *previewHandler) NotFoundPagePreview(c *fiber.Ctx) error {
	content := c.FormValue("content")
	layout := c.FormValue("layout")
	if content == "" && layout == "" {
		_, content = localizedNotFoundDefaults(c)
	}

	contractID, layoutID, err := bindingValue(layout)
	if err != nil {
		return err
	}

	output, diagnostics := sitePagePreviewBound(c.UserContext(), h.sitePageService, 0, content, contractID, layoutID)

	return Render(c, "partials/editor-preview", fiber.Map{
		"Output":      output,
		"Diagnostics": diagnostics,
	})
}
