package handler

import (
	"context"
	"html/template"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
)

// TODO: Refactor: remove h.build* funcs
type adminHandler struct {
	sitePageService  *service.SitePageService
	componentService *service.ComponentService
	themeService     *service.ThemeService
}

func NewAdminHandler(sps *service.SitePageService, cs *service.ComponentService, ts *service.ThemeService) *adminHandler {
	return &adminHandler{
		sitePageService:  sps,
		componentService: cs,
		themeService:     ts,
	}
}

func (h *adminHandler) Index(c *fiber.Ctx) error {
	sitePages, _ := h.sitePageService.GetPage(0)
	translator := c.Locals("I18n")

	var target string
	var sitePage models.SitePage

	if len(sitePages) > 0 {
		target = "/admin/site-pages/" + strconv.FormatUint(uint64(sitePages[0].ID), 10)
		sitePage = sitePages[0]
	} else {
		target = "/admin/site-pages/new"
	}

	if c.Get("HX-Request") != "true" {
		return c.Redirect(target)
	}

	c.Set("HX-Push-Url", target)

	return Render(c, "pages/admin", fiber.Map{
		"SitePageEditor": h.buildSitePageEditor(c.UserContext(), sitePage, translator),
		"SitePageUl":     sitePageList(h.sitePageService, 0, sitePage.ID),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	})
}

func (h *adminHandler) RenderAdminSitePage(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.SendStatus(400)
	}

	sitePage, err := h.sitePageService.GetByID(uint(id))
	if err != nil {
		return c.SendStatus(404)
	}

	return Render(c, "pages/admin", fiber.Map{
		"SitePageEditor": h.buildSitePageEditor(c.UserContext(), sitePage, c.Locals("I18n")),
		"SitePageUl":     sitePageList(h.sitePageService, 0, sitePage.ID),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	}, "layouts/admin")
}

func (h *adminHandler) RenderAdminSitePageEditor(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.SendStatus(400)
	}

	sitePage, err := h.sitePageService.GetByID(uint(id))
	if id != 0 && err != nil {
		return c.SendStatus(404)
	}

	return Render(c, "partials/htmx/site-page-editor", fiber.Map{
		"SitePageEditor": h.buildSitePageEditor(c.UserContext(), sitePage, c.Locals("I18n")),
		"SitePageUl":     sitePageList(h.sitePageService, 0, sitePage.ID),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	})
}

func (h *adminHandler) RenderAdminComponent(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.SendStatus(400)
	}

	comp, err := h.componentService.GetByID(uint(id))
	if err != nil {
		return c.SendStatus(404)
	}

	return Render(c, "pages/admin", fiber.Map{
		"ComponentMode":   true,
		"ComponentEditor": h.buildComponentEditor(c.UserContext(), comp, c.Locals("I18n")),
		"ComponentUl":     componentList(h.componentService, 0, comp.ID),
		"SitePageUl":      sitePageList(h.sitePageService, 0, 0),
	}, "layouts/admin")
}

// RenderSitePageList renders the page of the site page sidebar at the offset
// query parameter, followed by the button for the next page.
func (h *adminHandler) RenderSitePageList(c *fiber.Ctx) error {
	offset := c.QueryInt("offset")
	active := c.QueryInt("active")
	if active < 0 {
		active = 0
	}

	return Render(c, "partials/htmx/site-page-list", fiber.Map{
		"SitePageUl": sitePageList(h.sitePageService, offset, uint(active)),
	})
}

// RenderComponentList renders the page of the component sidebar at the offset
// query parameter, followed by the button for the next page.
func (h *adminHandler) RenderComponentList(c *fiber.Ctx) error {
	offset := c.QueryInt("offset")
	active := c.QueryInt("active")
	if active < 0 {
		active = 0
	}

	return Render(c, "partials/htmx/component-list", fiber.Map{
		"ComponentUl": componentList(h.componentService, offset, uint(active)),
	})
}

func (h *adminHandler) RenderAdminComponentEditor(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.SendStatus(400)
	}

	comp, err := h.componentService.GetByID(uint(id))
	if id != 0 && err != nil {
		return c.SendStatus(404)
	}

	return Render(c, "partials/htmx/component-editor", fiber.Map{
		"ComponentEditor": h.buildComponentEditor(c.UserContext(), comp, c.Locals("I18n")),
		"ComponentUl":     componentList(h.componentService, 0, comp.ID),
		"SitePageUl":      sitePageList(h.sitePageService, 0, 0),
	})
}

type sitePageEditor struct {
	ID             uint
	Name           string
	Slug           string
	Content        string
	Output         template.HTML
	Diagnostics    []view.DiagnosticItem
	IsEnabled      bool
	LastModifiedAt string
	SlugErr        string
	NameErr        string
	Layout         view.LayoutField
	PublicURL      string
	I18n           any
}

func (h *adminHandler) buildSitePageEditor(ctx context.Context, page models.SitePage, translator any) sitePageEditor {
	output, diagnostics := sitePagePreviewBound(ctx, h.sitePageService, page.ID, page.Content, page.ContractID, page.LayoutID)
	editor := sitePageEditor{
		ID:             page.ID,
		Name:           page.Name,
		Slug:           page.Slug,
		Content:        page.Content,
		Output:         output,
		Diagnostics:    diagnostics,
		IsEnabled:      page.Enabled,
		LastModifiedAt: view.RelativeTimeWithTranslator(page.LastModifiedAt, translator),
		Layout:         layoutField(translator, h.themeService, page, bindingString(page.ContractID, page.LayoutID)),
		I18n:           translator,
	}
	if page.ID != 0 {
		editor.PublicURL = "/" + page.Slug
	}
	return editor
}

func (h *adminHandler) buildComponentEditor(ctx context.Context, comp models.Component, translator any) view.ComponentEditorData {
	output, diagnostics := componentPreview(ctx, h.componentService, comp.Name, comp.Content)
	editor := view.ComponentEditor(comp, output, diagnostics, "", translator)
	if comp.ID != 0 {
		editor.PageErrorCount = h.sitePageService.PublishedPageCountOfGlobal(comp.Name)
	}
	return editor
}
