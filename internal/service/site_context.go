package service

import (
	"errors"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/umono/internal/repository"
)

// ErrInvalidSiteContextKey is returned when a site context key does not follow
// the site context key grammar.
var ErrInvalidSiteContextKey = errors.New("invalid site context key")

// ErrInvalidSiteContextValue is returned when the source of a site context
// does not parse as a value. The diagnostics that report why it does not
// parse are returned with it.
var ErrInvalidSiteContextValue = errors.New("invalid site context value")

// siteContextKeyPattern matches the site context keys. A site context key
// starts with site/ and its segments follow Compono's context key grammar, so
// the key can be written in context(...). Umono's own keys are outside site/.
var siteContextKeyPattern = regexp.MustCompile(`^site(/[a-z0-9]+(-[a-z0-9]+)*)+$`)

// SiteContextService keeps the values of the site contexts of the site. The
// values are parsed from the sources of the site contexts and are read by the
// renders.
type SiteContextService struct {
	// writeMu serializes changes to the values. Reads do not take it.
	writeMu sync.Mutex
	// values points to an immutable map of the site context values by key.
	// A change builds a new map and swaps it in.
	values atomic.Pointer[map[string]any]

	repo *repository.SiteContextRepository
}

// NewSiteContextService loads the site contexts and parses their sources. A
// source that does not parse is left out of the values.
func NewSiteContextService(repo *repository.SiteContextRepository) (*SiteContextService, error) {
	rows, err := repo.GetAll()
	if err != nil {
		return nil, err
	}

	values := map[string]any{}
	for _, row := range rows {
		value, diagnostics := compono.ParseValue([]byte(row.Source))
		if len(diagnostics) > 0 {
			continue
		}
		values[row.Key] = value
	}

	s := &SiteContextService{repo: repo}
	s.values.Store(&values)

	return s, nil
}

// Save saves the source of the site context of key and parses it. A source
// that does not parse is not saved: the row and the value of the key keep
// the last valid value. The diagnostics that report why the source does not
// parse are returned with ErrInvalidSiteContextValue. Their ranges are
// relative to the source.
func (s *SiteContextService) Save(key, source string) ([]compono.Diagnostic, error) {
	if !siteContextKeyPattern.MatchString(key) {
		return nil, ErrInvalidSiteContextKey
	}

	value, diagnostics := compono.ParseValue([]byte(source))
	if len(diagnostics) > 0 {
		return diagnostics, ErrInvalidSiteContextValue
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.Save(key, source); err != nil {
		return nil, err
	}

	s.updateValues(func(values map[string]any) {
		values[key] = value
	})

	return nil, nil
}

// Values returns the current values of the site contexts. Callers must not
// modify the map.
func (s *SiteContextService) Values() map[string]any {
	return *s.values.Load()
}

// updateValues applies change to a copy of the values and swaps the copy in.
// The caller must hold writeMu.
func (s *SiteContextService) updateValues(change func(values map[string]any)) {
	current := *s.values.Load()
	updated := make(map[string]any, len(current)+1)
	for key, value := range current {
		updated[key] = value
	}
	change(updated)
	s.values.Store(&updated)
}
