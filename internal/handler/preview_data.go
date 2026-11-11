package handler

import (
	"context"
	"html/template"
	"strings"

	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/view"
)

// sitePagePreviewBound renders source as the preview of the page editor with
// an unsaved layout binding. The preview marks the links to the address of
// the saved page with the pageID.
func sitePagePreviewBound(ctx context.Context, s *service.SitePageService, pageID uint, source string, contractID *uint, layoutID *string) (template.HTML, []view.DiagnosticItem) {
	output, diagnostics, _ := s.PreviewBound(ctx, pageID, source, contractID, layoutID)
	return mustPreviewHTML(output), view.EditorDiagnostics(diagnostics, nil)
}

// componentPreview renders source as the preview of the global editor of name.
func componentPreview(ctx context.Context, s *service.ComponentService, name, source string) (template.HTML, []view.DiagnosticItem) {
	name = strings.TrimSpace(name)
	output, diagnostics := s.MustPreview(ctx, name, source)
	return mustPreviewHTML(output), view.EditorDiagnostics(diagnostics, []string{name})
}
