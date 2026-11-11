package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
)

type SitePageHandler struct {
	sitePageService  *service.SitePageService
	componentService *service.ComponentService
	themeService     *service.ThemeService
}

func NewSitePageHandler(sps *service.SitePageService, cs *service.ComponentService, ts *service.ThemeService) *SitePageHandler {
	return &SitePageHandler{
		sitePageService:  sps,
		componentService: cs,
		themeService:     ts,
	}
}

func (h *SitePageHandler) Create(c *fiber.Ctx) error {
	// TODO: Add validator

	contractID, layoutID, err := bindingValue(c.FormValue("layout"))
	if err != nil {
		return err
	}

	sitePage := models.SitePage{
		Name:       c.FormValue("name"),
		Slug:       c.FormValue("slug"),
		Content:    c.FormValue("content"),
		Enabled:    c.FormValue("enabled") == "1",
		ContractID: contractID,
		LayoutID:   layoutID,
	}

	created, errs := h.sitePageService.Create(sitePage)

	slugErr := ""
	nameErr := ""
	saved := models.SitePage{}
	if len(errs) > 0 {
		if err := service.ErrInvalidSlug; service.ErrorsIs(errs, err) {
			slugErr = translatedValidationError(c, err)
		} else if err := service.ErrSlugAlreadyExists; service.ErrorsIs(errs, err) {
			slugErr = translatedValidationError(c, err)
		}
		if err := service.ErrNameRequired; service.ErrorsIs(errs, err) {
			nameErr = translatedValidationError(c, err)
		}
	} else {
		c.Set("HX-Push-Url", "/admin/site-pages/"+strconv.FormatUint(uint64(created.ID), 10))
		sitePage = created
		saved = created
	}

	output, diagnostics := sitePagePreviewBound(c.UserContext(), h.sitePageService, saved.ID, sitePage.Content, sitePage.ContractID, sitePage.LayoutID)
	editor := view.SitePageEditor(sitePage, output, diagnostics, slugErr, nameErr, c.Locals("I18n"))
	editor.Layout = layoutField(c.Locals("I18n"), h.themeService, saved, c.FormValue("layout"))
	editor.Layout.Error, editor.Layout.Skeleton = bindingErrorText(c, errs)
	if saved.ID != 0 {
		editor.PublicURL = "/" + saved.Slug
	}
	return Render(c, "partials/htmx/site-page-editor", fiber.Map{
		"SitePageEditor": editor,
		"SitePageUl":     sitePageList(h.sitePageService, 0, sitePage.ID),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	})
}

func (h *SitePageHandler) Update(c *fiber.Ctx) error {
	// TODO: Add validator
	u64, _ := strconv.ParseUint(c.FormValue("id"), 10, 0)

	contractID, layoutID, err := bindingValue(c.FormValue("layout"))
	if err != nil {
		return err
	}

	saved, _ := h.sitePageService.GetByID(uint(u64))

	sitePage := models.SitePage{
		ID:         uint(u64),
		Name:       c.FormValue("name"),
		Slug:       c.FormValue("slug"),
		Content:    c.FormValue("content"),
		Enabled:    c.FormValue("enabled") == "1",
		ContractID: contractID,
		LayoutID:   layoutID,
	}

	updated, errs := h.sitePageService.Update(sitePage)

	slugErr := ""
	nameErr := ""
	if len(errs) > 0 {
		if err := service.ErrInvalidSlug; service.ErrorsIs(errs, err) {
			slugErr = translatedValidationError(c, err)
		} else if err := service.ErrSlugAlreadyExists; service.ErrorsIs(errs, err) {
			slugErr = translatedValidationError(c, err)
		}
		if err := service.ErrNameRequired; service.ErrorsIs(errs, err) {
			nameErr = translatedValidationError(c, err)
		}
	} else {
		sitePage = updated
		saved = updated
	}

	output, diagnostics := sitePagePreviewBound(c.UserContext(), h.sitePageService, saved.ID, sitePage.Content, sitePage.ContractID, sitePage.LayoutID)
	editor := view.SitePageEditor(sitePage, output, diagnostics, slugErr, nameErr, c.Locals("I18n"))
	editor.Layout = layoutField(c.Locals("I18n"), h.themeService, saved, c.FormValue("layout"))
	editor.Layout.Error, editor.Layout.Skeleton = bindingErrorText(c, errs)
	if saved.ID != 0 {
		editor.PublicURL = "/" + saved.Slug
	}
	return Render(c, "partials/htmx/site-page-editor", fiber.Map{
		"SitePageEditor": editor,
		"SitePageUl":     sitePageList(h.sitePageService, 0, sitePage.ID),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	})
}

func (h *SitePageHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := h.sitePageService.Delete(uint(id)); err != nil {
		return err
	}

	c.Set("HX-Redirect", "/admin/site-pages/new")
	return c.SendStatus(fiber.StatusNoContent)
}

// SaveNotFoundPage saves the 404 page from its settings screen.
func (h *SitePageHandler) SaveNotFoundPage(c *fiber.Ctx) error {
	contractID, layoutID, err := bindingValue(c.FormValue("layout"))
	if err != nil {
		return err
	}
	if err := h.sitePageService.SaveNotFoundPage(c.FormValue("title"), c.FormValue("content"), contractID, layoutID); err != nil {
		text, skeleton := bindingErrorText(c, []error{err})
		if text == "" {
			return err
		}
		c.Set("HX-Retarget", "#not-found-layout-error")
		c.Set("HX-Reswap", "innerHTML")
		return Render(c, "partials/layout-error", fiber.Map{"Error": text, "Skeleton": skeleton})
	}

	c.Set("HX-Trigger", "notFoundPageOptionSaved")
	return c.SendStatus(fiber.StatusOK)
}

// LayoutInfo renders the info of the Layout field's chosen option: its
// parameters and notes. current is the page's saved binding, so that its old
// record is known.
func (h *SitePageHandler) LayoutInfo(c *fiber.Ctx) error {
	contractID, layoutID, err := bindingValue(c.Query("current"))
	if err != nil {
		return err
	}
	options, err := h.themeService.BindingOptions(models.SitePage{ContractID: contractID, LayoutID: layoutID})
	if err != nil {
		return err
	}
	return Render(c, "partials/layout-info", fiber.Map{
		"Info": view.NewLayoutInfo(options, c.Query("layout"), c.Locals("I18n")),
	})
}

func (h *SitePageHandler) RenderNewPageSiteEditor(c *fiber.Ctx) error {
	editor := view.SitePageEditor(models.SitePage{}, mustPreviewHTML(""), nil, "", "", c.Locals("I18n"))
	editor.Layout = layoutField(c.Locals("I18n"), h.themeService, models.SitePage{}, "")
	return Render(c, "pages/admin", fiber.Map{
		"SitePageEditor": editor,
		"SitePageUl":     sitePageList(h.sitePageService, 0, 0),
		"ComponentUl":    componentList(h.componentService, 0, 0),
	}, "layouts/admin")
}

// bindingValue parses the Layout field's value, "<record ID>:<layout id>",
// into a binding. An empty value is no binding.
func bindingValue(v string) (*uint, *string, error) {
	if v == "" {
		return nil, nil, nil
	}
	id, layout, ok := strings.Cut(v, ":")
	if !ok || layout == "" {
		return nil, nil, fiber.ErrBadRequest
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, nil, fiber.ErrBadRequest
	}
	contractID := uint(n)
	return &contractID, &layout, nil
}
