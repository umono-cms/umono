package handler

import (
	"path"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
)

type settingsHandler struct {
	settingsService *service.SettingsService
	optionService   *service.OptionService
	sitePageService *service.SitePageService
	storageService  *service.StorageService
	themeService    *service.ThemeService
}

func NewSettingsHandler(ss *service.SettingsService, os *service.OptionService, sps *service.SitePageService, sts *service.StorageService, ts *service.ThemeService) *settingsHandler {
	return &settingsHandler{
		settingsService: ss,
		optionService:   os,
		sitePageService: sps,
		storageService:  sts,
		themeService:    ts,
	}
}

func (h *settingsHandler) Index(c *fiber.Ctx) error {
	menuItems := h.settingsService.MenuItems()
	if len(menuItems) == 0 {
		return c.SendStatus(fiber.StatusOK)
	}

	return c.Redirect("/admin/settings/" + menuItems[0].Slug)
}

func (h *settingsHandler) Render404Page(c *fiber.Ctx) error {
	partial := "partials/settings-404-page"
	layouts := []string{"layouts/settings", "layouts/admin"}

	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-404-page"
		layouts = []string{}
	}

	var title, content string
	page, err := h.sitePageService.GetNotFoundPageSource()
	if err == nil {
		title, content = page.Name, page.Content
	}
	defaultTitle, defaultContent := localizedNotFoundDefaults(c)

	previewContent := content
	if content == "" && page.ContractID == nil {
		previewContent = defaultContent
	}

	output, diagnostics := sitePagePreviewBound(c.UserContext(), h.sitePageService, 0, previewContent, page.ContractID, page.LayoutID)
	return Render(c, partial, fiber.Map{
		"SettingsUl":     h.settingsUl(c, path.Base(c.Path())),
		"DefaultTitle":   defaultTitle,
		"DefaultContent": defaultContent,
		"Title":          title,
		"Content":        content,
		"Output":         output,
		"Diagnostics":    diagnostics,
		"Layout":         layoutField(c.Locals("I18n"), h.themeService, page, bindingString(page.ContractID, page.LayoutID)),
	}, layouts...)
}

func (h *settingsHandler) RenderRobotsTxt(c *fiber.Ctx) error {
	partial := "partials/settings-robots-txt"
	layouts := []string{"layouts/settings", "layouts/admin"}

	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-robots-txt"
		layouts = []string{}
	}

	return Render(c, partial, fiber.Map{
		"SettingsUl": h.settingsUl(c, path.Base(c.Path())),
		"Content":    h.optionService.GetRobotsTxt(),
	}, layouts...)
}

func (h *settingsHandler) RenderGeneral(c *fiber.Ctx) error {
	partial := "partials/settings-general"
	layouts := []string{"layouts/settings", "layouts/admin"}

	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-general"
		layouts = []string{}
	}

	return Render(c, partial, fiber.Map{
		"SettingsUl": h.settingsUl(c, path.Base(c.Path())),
		"Language":   h.optionService.GetLanguage(),
		"Languages":  h.optionService.SupportedLanguages(),
		"SiteURL":    h.optionService.GetSiteURL(),
		"SitemapURL": sitemapURLOf(h.optionService.GetSiteURL()),
	}, layouts...)
}

func (h *settingsHandler) RenderAbout(c *fiber.Ctx) error {
	partial := "partials/settings-about"
	layouts := []string{"layouts/settings", "layouts/admin"}

	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-about"
		layouts = []string{}
	}

	return Render(c, partial, fiber.Map{
		"SettingsUl": h.settingsUl(c, path.Base(c.Path())),
	}, layouts...)
}

func (h *settingsHandler) settingsUl(c *fiber.Ctx, activeSlug string) []view.SettingsLi {
	ul := []view.SettingsLi{}
	translator, _ := c.Locals("I18n").(interface{ T(string) string })
	for _, mi := range h.settingsService.MenuItems() {
		title := mi.TitleKey
		if translator != nil {
			title = translator.T(mi.TitleKey)
		}

		ul = append(ul, view.SettingsLi{
			Title:    title,
			Slug:     mi.Slug,
			IsActive: mi.Slug == activeSlug,
		})
	}
	return ul
}

func isSettingsContentSwap(c *fiber.Ctx) bool {
	if c.Get("HX-Request") != "true" {
		return false
	}

	switch c.Get("HX-Target") {
	case "settings-content", "settings-general-content", "settings-404-page-content", "settings-robots-txt-content", "settings-about-content", "settings-storage-content", "settings-themes-content", "settings-theme-pages-content":
		return true
	default:
		return false
	}
}

// sitemapURLOf returns the address of sitemap.xml under siteURL, or "" when
// the site address is not set.
func sitemapURLOf(siteURL string) string {
	if siteURL == "" {
		return ""
	}
	return siteURL + "/sitemap.xml"
}
