package handler

import (
	"bytes"
	"errors"
	"html/template"
	"strings"

	umono "github.com/umono-cms/umono"
	"github.com/umono-cms/umono/internal/service"

	"github.com/gofiber/fiber/v2"
)

// viewData adds the values every view gets to data.
func viewData(c *fiber.Ctx, data fiber.Map) fiber.Map {
	if data == nil {
		data = fiber.Map{}
	}
	data["I18n"] = c.Locals("I18n")
	data["Lang"] = c.Locals("Lang")
	data["Dir"] = c.Locals("Dir")
	data["IsHTMX"] = c.Locals("IsHTMX")
	data["Version"] = umono.Version
	data["IsAlreadyOnSettings"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/settings")
	data["IsAlreadyOnSystem"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/system")
	data["IsAlreadyOnSitePages"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/site-pages")
	data["IsAlreadyOnComponents"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/components")
	data["IsAlreadyOnMedia"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/media")
	data["IsAlreadyOnContext"] = strings.Contains(c.Get("HX-Current-URL"), "/admin/context")
	return data
}

func Render(c *fiber.Ctx, view string, data fiber.Map, layouts ...string) error {
	return c.Render(view, viewData(c, data), layouts...)
}

// renderBytes renders view without a layout with the values every view gets
// and returns the output.
func renderBytes(c *fiber.Ctx, view string, data fiber.Map) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.App().Config().Views.Render(&buf, view, viewData(c, data)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func localizedNotFoundDefaults(c *fiber.Ctx) (string, string) {
	translator, _ := c.Locals("I18n").(interface{ T(string) string })
	if translator == nil {
		return "Page Not Found", "# Page Not Found\nThe page you're looking for may have been moved, deleted, or never existed."
	}

	return translator.T("settings.404_page.default_title"), translator.T("settings.404_page.default_content")
}

func translate(c *fiber.Ctx, key string) string {
	translator, _ := c.Locals("I18n").(interface{ T(string) string })
	if translator == nil {
		return key
	}

	return translator.T(key)
}

func translatedValidationError(c *fiber.Ctx, err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidSlug):
		return translate(c, "admin.pages.errors.invalid_slug")
	case errors.Is(err, service.ErrSlugAlreadyExists):
		return translate(c, "admin.pages.errors.slug_exists")
	case errors.Is(err, service.ErrNameRequired):
		return translate(c, "common.errors.name_required")
	case errors.Is(err, service.ErrInvalidComponentName):
		return translate(c, "admin.components.errors.invalid_name")
	case errors.Is(err, service.ErrComponentNameAlreadyExists):
		return translate(c, "admin.components.errors.name_exists")
	case errors.Is(err, service.ErrGlobalCallsLayout):
		return translate(c, "admin.components.errors.calls_layout")
	case errors.Is(err, service.ErrStorageNameRequired):
		return translate(c, "settings.storage.errors.name_required")
	case errors.Is(err, service.ErrStorageEndpointRequired):
		return translate(c, "settings.storage.errors.endpoint_required")
	case errors.Is(err, service.ErrStorageRegionRequired):
		return translate(c, "settings.storage.errors.region_required")
	case errors.Is(err, service.ErrStorageBucketRequired):
		return translate(c, "settings.storage.errors.bucket_required")
	case errors.Is(err, service.ErrStorageAccessKeyRequired):
		return translate(c, "settings.storage.errors.access_key_required")
	case errors.Is(err, service.ErrStorageSecretKeyRequired):
		return translate(c, "settings.storage.errors.secret_key_required")
	case errors.Is(err, service.ErrStorageTestBodyMismatch):
		return translate(c, "settings.storage.test.errors.body_mismatch")
	case errors.Is(err, service.ErrInvalidSiteContextKey):
		return translate(c, "context.errors.invalid_key")
	case errors.Is(err, service.ErrSiteContextKeyAlreadyExists):
		return translate(c, "context.errors.key_exists")
	case errors.Is(err, service.ErrInvalidSiteContextValue):
		return translate(c, "context.errors.invalid_value")
	default:
		return err.Error()
	}
}

func mustPreviewHTML(renderedHTML string) template.HTML {
	return template.HTML(renderedHTML)
}
