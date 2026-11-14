package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/view"
)

// hostPlaceholder is the part of a translated text that carries the host of
// the registry.
const hostPlaceholder = "{{host}}"

// RenderThemes renders the theme management page.
func (h *settingsHandler) RenderThemes(c *fiber.Ctx) error {
	return h.renderThemes(c, "", "")
}

// renderThemes renders the theme management page with the outcome of an
// action: message on success, failure on failure.
func (h *settingsHandler) renderThemes(c *fiber.Ctx, message, failure string) error {
	partial := "partials/settings-themes"
	layouts := []string{"layouts/settings", "layouts/admin"}
	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-themes"
		layouts = []string{}
	}
	list, err := h.themeService.List()
	if err != nil {
		return err
	}
	return Render(c, partial, fiber.Map{
		"SettingsUl":              h.settingsUl(c, "themes"),
		"ThemesPage":              view.NewThemesPage(list),
		"Message":                 message,
		"Failure":                 failure,
		"ActivateConfirmRegistry": h.translateHost(c, "settings.themes.activate_confirm_registry"),
		"ActivateConfirmStored":   h.translateHost(c, "settings.themes.activate_confirm_stored"),
	}, layouts...)
}

// translateHost translates key and puts the host of the registry in place of
// its {{host}} placeholder.
func (h *settingsHandler) translateHost(c *fiber.Ctx, key string) string {
	return strings.ReplaceAll(translate(c, key), hostPlaceholder, h.themeService.RegistryHost())
}

// RenderThemePages lists the pages of a group bound to a newer minor than the
// contract reference in the query, so the site owner can find what keeps a
// theme from being selected. A bad reference goes back to the themes screen.
func (h *settingsHandler) RenderThemePages(c *fiber.Ctx) error {
	ref, err := models.ParseContractRef(c.Query("contract"))
	if err != nil {
		return c.Redirect("/admin/settings/themes", fiber.StatusSeeOther)
	}
	partial := "partials/settings-theme-pages"
	layouts := []string{"layouts/settings", "layouts/admin"}
	if isSettingsContentSwap(c) {
		partial = "partials/htmx/settings-theme-pages"
		layouts = []string{}
	}
	return Render(c, partial, fiber.Map{
		"SettingsUl": h.settingsUl(c, "themes"),
		"ThemePages": view.NewThemePagesView(ref, h.themeService.UncoveredPages(ref)),
	}, layouts...)
}

// failure joins the translated headline of a failed action and its reason.
// An error that wraps themecore.ErrOutdated is shown as the out of date
// message instead of its raw text.
func failure(c *fiber.Ctx, key string, err error) string {
	if errors.Is(err, themecore.ErrOutdated) {
		return translate(c, key) + ": " + translate(c, "settings.themes.outdated")
	}
	return translate(c, key) + ": " + err.Error()
}

// ActivateTheme activates the installed theme at the form's path.
func (h *settingsHandler) ActivateTheme(c *fiber.Ctx) error {
	candidate, err := h.themeService.Activate(c.UserContext(), c.FormValue("path"))
	if err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.activate_failed", err))
	}
	note := "settings.themes.verified_stored"
	if candidate.Asked {
		note = "settings.themes.verified_registry"
	}
	return h.renderThemes(c, translate(c, "settings.themes.activated")+" "+h.translateHost(c, note), "")
}

// DeactivateTheme deactivates the active theme of the group of the form's
// contract.
func (h *settingsHandler) DeactivateTheme(c *fiber.Ctx) error {
	ref, err := models.ParseContractRef(c.FormValue("contract"))
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if err := h.themeService.Deactivate(ref.Group()); err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.deactivate_failed", err))
	}
	return h.renderThemes(c, translate(c, "settings.themes.deactivated"), "")
}

// RetryTheme reruns the checks of loading for the broken active theme of the
// group of the form's contract.
func (h *settingsHandler) RetryTheme(c *fiber.Ctx) error {
	ref, err := models.ParseContractRef(c.FormValue("contract"))
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if err := h.themeService.Retry(c.UserContext(), ref.Group()); err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.retry_failed", err))
	}
	return h.renderThemes(c, translate(c, "settings.themes.retried"), "")
}

// ApplyPending uses the new content of the pending install at the form's
// path.
func (h *settingsHandler) ApplyPending(c *fiber.Ctx) error {
	if err := h.themeService.ApplyPending(c.UserContext(), c.FormValue("path")); err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.apply_failed", err))
	}
	return h.renderThemes(c, translate(c, "settings.themes.applied"), "")
}

// RejectPending deletes the pending install at the form's path.
func (h *settingsHandler) RejectPending(c *fiber.Ctx) error {
	if err := h.themeService.RejectPending(c.FormValue("path")); err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.reject_failed", err))
	}
	return h.renderThemes(c, translate(c, "settings.themes.rejected"), "")
}

// DeleteContractRecord deletes the stored record of the form's contract.
func (h *settingsHandler) DeleteContractRecord(c *fiber.Ctx) error {
	ref, err := models.ParseContractRef(c.FormValue("contract"))
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if err := h.themeService.DeleteContract(ref); err != nil {
		return h.renderThemes(c, "", failure(c, "settings.themes.delete_failed", err))
	}
	return h.renderThemes(c, translate(c, "settings.themes.record_deleted"), "")
}
