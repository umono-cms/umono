package service

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
)

var (
	ErrSlugAlreadyExists = errors.New("Slug already exists for another page")
	ErrInvalidSlug       = errors.New("Invalid slug")
	ErrNameRequired      = errors.New("Name required")
)

var ErrSitePageNotFound = errors.New("site page not found")

// ErrSystemPage is returned when a system page is deleted or saved as an
// ordinary page.
var ErrSystemPage = errors.New("system page")

// PageThemes resolves how a page renders with the themes and checks its
// layout binding before it is saved. ThemeService is one.
type PageThemes interface {
	PageRender(page models.SitePage) (PageRender, error)
	CheckBinding(page models.SitePage, previous *models.SitePage) error
}

// RenderedPage is a page rendered for a visitor. Content holds <umono-root> with the Compono output.
type RenderedPage struct {
	models.SitePage
	// ImageURLs are the image URLs Compono wrote to the output.
	ImageURLs []string
	// Release is the active theme's release, "" when the page renders with
	// the fallback.
	Release string
}

type SitePageService struct {
	repo            *repository.SitePageRepository
	optRepo         *repository.OptionRepository
	contentCompiler *ContentCompiler
	diagnosticRepo  *repository.PageDiagnosticRepository
	pageThemes      PageThemes
	// diagnosticMu serializes the recomputations of the stored diagnostics so
	// that the last one writes what the latest inputs render.
	diagnosticMu sync.Mutex
}

func NewSitePageService(r *repository.SitePageRepository, or *repository.OptionRepository, cc *ContentCompiler) *SitePageService {
	return &SitePageService{
		repo:            r,
		optRepo:         or,
		contentCompiler: cc,
	}
}

// GetRenderedBySlug renders the enabled page with the slug for a visitor.
func (s *SitePageService) GetRenderedBySlug(ctx context.Context, slug string) (RenderedPage, error) {
	// TODO: Put here cache
	sitePage, err := s.GetBySlug(slug)
	if err != nil {
		return RenderedPage{}, err
	}

	if !sitePage.Enabled {
		return RenderedPage{}, ErrSitePageNotFound
	}

	r := s.pageRender(sitePage)
	r.Current = currentPage(pageAddress(sitePage.Slug))
	root, imageURLs, _, err := s.contentCompiler.RenderRoot(ctx, sitePage.Content, r)
	if err != nil {
		return RenderedPage{}, err
	}
	sitePage.Content = root
	return RenderedPage{SitePage: sitePage, ImageURLs: imageURLs, Release: r.Release}, nil
}

// GetNotFoundPage renders the 404 page for a visitor. The localized default
// content is used only when the 404 page has no binding.
func (s *SitePageService) GetNotFoundPage(ctx context.Context, defaultTitle, defaultContent string) (RenderedPage, error) {
	title := defaultTitle
	content := defaultContent
	page := s.repo.GetBySystemKey(models.NotFoundSystemKey)
	if page.ID != 0 {
		if page.Name != "" {
			title = page.Name
		}
		if page.Content != "" || page.ContractID != nil {
			content = page.Content
		}
	}

	r := s.pageRender(page)
	root, imageURLs, _, err := s.contentCompiler.RenderRoot(ctx, content, r)
	if err != nil {
		return RenderedPage{}, err
	}

	return RenderedPage{SitePage: models.SitePage{Name: title, Content: root}, ImageURLs: imageURLs, Release: r.Release}, nil
}

// GetNotFoundPageSource returns the stored 404 system page, unrendered.
func (s *SitePageService) GetNotFoundPageSource() (models.SitePage, error) {
	page := s.repo.GetBySystemKey(models.NotFoundSystemKey)
	if page.ID == 0 {
		return models.SitePage{}, ErrSitePageNotFound
	}
	return page, nil
}

// SaveNotFoundPage saves the name, the content and the layout binding of the
// 404 system page, after the binding's save checks, and recomputes its
// diagnostics. An empty name or, without a binding, an empty content makes a
// visitor see the localized defaults.
func (s *SitePageService) SaveNotFoundPage(name, content string, contractID *uint, layoutID *string) error {
	page, err := s.GetNotFoundPageSource()
	if err != nil {
		return err
	}
	if s.pageThemes != nil {
		candidate := page
		candidate.Name, candidate.Content = name, content
		candidate.ContractID, candidate.LayoutID = contractID, layoutID
		if err := s.pageThemes.CheckBinding(candidate, &page); err != nil {
			return err
		}
	}
	if err := s.repo.UpdateSystemPage(page.ID, name, content, contractID, layoutID, time.Now()); err != nil {
		return err
	}
	if err := s.RecomputeDiagnostics(context.Background(), page.ID); err != nil {
		log.Printf("recompute diagnostics of page %d: %v", page.ID, err)
	}
	return nil
}

func (s *SitePageService) GetBySlug(slug string) (models.SitePage, error) {
	sp := s.repo.GetBySlug(slug)
	if sp.ID == 0 {
		return models.SitePage{}, ErrSitePageNotFound
	}
	return sp, nil
}

func (s *SitePageService) GetByID(ID uint) (models.SitePage, error) {
	sp := s.repo.GetByID(ID)
	if sp.ID == 0 || sp.SystemKey != nil {
		return models.SitePage{}, ErrSitePageNotFound
	}
	return sp, nil
}

func (s *SitePageService) GetAll() []models.SitePage {
	return s.repo.GetAll()
}

// GetPage returns a page of the pages as the admin sidebar lists them and the offset of the next page, or 0 when there is none.
func (s *SitePageService) GetPage(offset int) ([]models.SitePage, int) {
	if offset < 0 {
		offset = 0
	}
	items := s.repo.GetPage(offset, listPageSize+1)
	return pageOf(items, offset)
}

func (s *SitePageService) Preview(ctx context.Context, source string) (string, []compono.Diagnostic, error) {
	return s.contentCompiler.CompileWithProviderContext(ctx, source)
}

func (s *SitePageService) MustPreview(ctx context.Context, source string) (string, []compono.Diagnostic) {
	output, diagnostics, _ := s.Preview(ctx, source)
	return output, diagnostics
}

// PreviewBound converts source for the editor's preview with an unsaved
// layout binding. The preview shows the structure: no theme stylesheet is
// applied to it. The preview marks the links to the address of the saved
// page with the pageID. The pageID 0, of a new page or of the 404 page, has
// no address, so that no link is marked.
func (s *SitePageService) PreviewBound(ctx context.Context, pageID uint, source string, contractID *uint, layoutID *string) (string, []compono.Diagnostic, error) {
	r := s.pageRender(models.SitePage{ContractID: contractID, LayoutID: layoutID})
	if pageID != 0 {
		if page, err := s.GetByID(pageID); err == nil {
			r.Current = currentPage(pageAddress(page.Slug))
		}
	}
	return s.contentCompiler.CompileWithProviderContext(ctx, source, r.Options()...)
}

func (s *SitePageService) Create(sp models.SitePage) (models.SitePage, []error) {
	errs := []error{}

	if !s.isSlugValid(sp.Slug) {
		errs = append(errs, ErrInvalidSlug)
	}

	if s.isSlugUsed(sp.Slug, 0) {
		errs = append(errs, ErrSlugAlreadyExists)
	}

	if sp.Name == "" {
		errs = append(errs, ErrNameRequired)
	}

	if s.pageThemes != nil {
		if err := s.pageThemes.CheckBinding(sp, nil); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return models.SitePage{}, errs
	}

	now := time.Now()
	sp.LastModifiedAt = &now

	created := s.repo.Create(sp)
	if err := s.RecomputeDiagnostics(context.Background(), created.ID); err != nil {
		log.Printf("recompute diagnostics of page %d: %v", created.ID, err)
	}
	return created, nil
}

func (s *SitePageService) Update(sp models.SitePage) (models.SitePage, []error) {
	if s.repo.GetByID(sp.ID).SystemKey != nil {
		return models.SitePage{}, []error{ErrSystemPage}
	}
	errs := []error{}

	if !s.isSlugValid(sp.Slug) {
		errs = append(errs, ErrInvalidSlug)
	}

	if s.isSlugUsed(sp.Slug, sp.ID) {
		errs = append(errs, ErrSlugAlreadyExists)
	}

	if sp.Name == "" {
		errs = append(errs, ErrNameRequired)
	}

	if s.pageThemes != nil {
		previous := s.repo.GetByID(sp.ID)
		if err := s.pageThemes.CheckBinding(sp, &previous); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return models.SitePage{}, errs
	}

	now := time.Now()
	sp.LastModifiedAt = &now

	updated := s.repo.Update(sp)
	if err := s.RecomputeDiagnostics(context.Background(), updated.ID); err != nil {
		log.Printf("recompute diagnostics of page %d: %v", updated.ID, err)
	}
	return updated, nil
}

func (s *SitePageService) Delete(id uint) error {
	if s.repo.GetByID(id).SystemKey != nil {
		return ErrSystemPage
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	if s.diagnosticRepo != nil {
		return s.diagnosticRepo.DeleteByPage(id)
	}
	return nil
}

func (s *SitePageService) CheckSlug(slug string, exclude uint) error {
	if !s.isSlugValid(slug) {
		return ErrInvalidSlug
	}
	if s.isSlugUsed(slug, exclude) {
		return ErrSlugAlreadyExists
	}
	return nil
}

// SetPageDiagnosticRepository sets where the diagnostics of the pages' last renders are stored.
func (s *SitePageService) SetPageDiagnosticRepository(r *repository.PageDiagnosticRepository) {
	s.diagnosticRepo = r
}

// SetPageThemes sets how the pages render with the themes. Without it every
// page renders without a layout and with the fallback.
func (s *SitePageService) SetPageThemes(t PageThemes) {
	s.pageThemes = t
}

// RecomputeDiagnostics renders the page with the id and replaces its stored diagnostics.
func (s *SitePageService) RecomputeDiagnostics(ctx context.Context, id uint) error {
	if s.diagnosticRepo == nil {
		return nil
	}
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	page := s.repo.GetByID(id)
	if page.ID == 0 {
		return nil
	}
	return s.recomputeDiagnostics(ctx, page)
}

// RecomputeAllDiagnostics renders every page and replaces its stored diagnostics.
func (s *SitePageService) RecomputeAllDiagnostics(ctx context.Context) error {
	if s.diagnosticRepo == nil {
		return nil
	}
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	for _, page := range s.repo.GetAll() {
		if err := s.recomputeDiagnostics(ctx, page); err != nil {
			return err
		}
	}
	return nil
}

// diagnosticsFilledKey is the option that records that FillDiagnosticsOnce has
// filled the stored diagnostics.
const diagnosticsFilledKey = "page_diagnostics_filled"

// FillDiagnosticsOnce recomputes the stored diagnostics of every page on the
// first launch that stores them, so that the errors of content saved before
// show up in the list. Later calls change nothing.
func (s *SitePageService) FillDiagnosticsOnce(ctx context.Context) error {
	if s.diagnosticRepo == nil || s.optRepo == nil {
		return nil
	}
	if s.optRepo.GetOptionByKey(diagnosticsFilledKey).ID != 0 {
		return nil
	}
	if err := s.RecomputeAllDiagnostics(ctx); err != nil {
		return err
	}
	return s.optRepo.SaveOption(diagnosticsFilledKey, "done")
}

// PublishedDiagnostics returns the stored diagnostics of the enabled pages.
func (s *SitePageService) PublishedDiagnostics() []models.PublishedDiagnostic {
	if s.diagnosticRepo == nil {
		return []models.PublishedDiagnostic{}
	}
	return s.diagnosticRepo.GetPublished()
}

// PublishedPageCountOfGlobal returns the number of enabled pages with a stored
// diagnostic whose source is the global name or below it.
func (s *SitePageService) PublishedPageCountOfGlobal(name string) int {
	if s.diagnosticRepo == nil {
		return 0
	}
	return int(s.diagnosticRepo.CountPublishedPagesBySource(name))
}

func (s *SitePageService) recomputeDiagnostics(ctx context.Context, page models.SitePage) error {
	_, diagnostics, err := s.contentCompiler.CompileWithProviderContext(ctx, page.Content, s.pageRender(page).Options()...)
	if err != nil {
		return err
	}
	items := make([]models.PageDiagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		items = append(items, models.PageDiagnostic{
			Code:    string(d.Code),
			Message: d.Message,
			Source:  strings.Join(d.Source, " > "),
		})
	}
	return s.diagnosticRepo.Replace(page.ID, items)
}

// pageRender returns how page renders. A binding that cannot be resolved is
// logged and the page renders without a layout, so that one broken binding
// does not stop the render of the others.
func (s *SitePageService) pageRender(page models.SitePage) PageRender {
	if s.pageThemes == nil {
		return PageRender{}
	}
	r, err := s.pageThemes.PageRender(page)
	if err != nil {
		log.Printf("error: render of page %d: %v", page.ID, err)
		return PageRender{}
	}
	return r
}

func (s *SitePageService) isSlugValid(slug string) bool {
	if slug == "" {
		return true
	}
	return regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(slug)
}

func (s *SitePageService) isSlugUsed(slug string, excluding uint) bool {
	used := s.repo.GetBySlug(slug)
	if used.ID == 0 || used.ID == excluding {
		return false
	}
	return true
}
