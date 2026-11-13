package service

import (
	"sync"
	"sync/atomic"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/umono/internal/repository"
)

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
// that does not parse removes the key from the values.
func (s *SiteContextService) Save(key, source string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.Save(key, source); err != nil {
		return err
	}

	value, diagnostics := compono.ParseValue([]byte(source))
	s.updateValues(func(values map[string]any) {
		if len(diagnostics) > 0 {
			delete(values, key)
			return
		}
		values[key] = value
	})

	return nil
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
