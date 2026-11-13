package service

import (
	"context"
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

// ErrSiteContextNotFound is returned when there is no site context of the
// key that is deleted or renamed.
var ErrSiteContextNotFound = errors.New("site context not found")

// ErrSiteContextKeyAlreadyExists is returned when a site context is renamed
// to a key that is already used by another site context.
var ErrSiteContextKeyAlreadyExists = errors.New("site context key already exists")

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
	// renderInputChanged is called after a change to the site contexts,
	// which can change the render of any page.
	renderInputChanged func(ctx context.Context)
}

// SiteContextService is a ContextProvider: the renders, the previews and the
// diagnostics see the same values, and a value that is saved is used by the
// next render.
var _ ContextProvider = (*SiteContextService)(nil)

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

	if err := s.save(key, source, value); err != nil {
		return nil, err
	}

	s.notifyRenderInputChange()
	return nil, nil
}

// save saves the source of the site context of key and swaps its value in.
// It holds writeMu for the write.
func (s *SiteContextService) save(key, source string, value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.Save(key, source); err != nil {
		return err
	}

	s.updateValues(func(values map[string]any) {
		values[key] = value
	})

	return nil
}

// Delete deletes the site context of key and its value. It returns
// ErrSiteContextNotFound when there is no site context of the key. The pages
// that use the key produce an unknown-key diagnostic from then on.
func (s *SiteContextService) Delete(key string) error {
	if err := s.delete(key); err != nil {
		return err
	}

	s.notifyRenderInputChange()
	return nil
}

// delete deletes the site context of key and its value. It holds writeMu for
// the write.
func (s *SiteContextService) delete(key string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	deleted, err := s.repo.Delete(key)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrSiteContextNotFound
	}

	s.updateValues(func(values map[string]any) {
		delete(values, key)
	})

	return nil
}

// Rename renames the site context of oldKey to newKey and moves its value
// to the new key. It returns ErrInvalidSiteContextKey when newKey does not
// follow the site context key grammar, ErrSiteContextKeyAlreadyExists when
// newKey is already used by another site context, and
// ErrSiteContextNotFound when there is no site context of oldKey. The pages
// that use the old key produce an unknown-key diagnostic from then on.
func (s *SiteContextService) Rename(oldKey, newKey string) error {
	if !siteContextKeyPattern.MatchString(newKey) {
		return ErrInvalidSiteContextKey
	}
	if oldKey == newKey {
		return nil
	}

	if err := s.rename(oldKey, newKey); err != nil {
		return err
	}

	s.notifyRenderInputChange()
	return nil
}

// rename renames the site context of oldKey to newKey and moves its value to
// the new key. It holds writeMu for the write.
func (s *SiteContextService) rename(oldKey, newKey string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	exists, err := s.repo.Exists(newKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrSiteContextKeyAlreadyExists
	}

	renamed, err := s.repo.Rename(oldKey, newKey)
	if err != nil {
		return err
	}
	if !renamed {
		return ErrSiteContextNotFound
	}

	s.updateValues(func(values map[string]any) {
		value, ok := values[oldKey]
		delete(values, oldKey)
		if !ok {
			return
		}
		values[newKey] = value
	})

	return nil
}

// OnRenderInputChange sets fn to be called after a site context is saved,
// deleted or renamed, which can change the render of any page.
func (s *SiteContextService) OnRenderInputChange(fn func(ctx context.Context)) {
	s.renderInputChanged = fn
}

// notifyRenderInputChange tells the listener that a site context changed.
func (s *SiteContextService) notifyRenderInputChange() {
	if s.renderInputChanged != nil {
		s.renderInputChanged(context.Background())
	}
}

// Values returns the current values of the site contexts. Callers must not
// modify the map.
func (s *SiteContextService) Values() map[string]any {
	return *s.values.Load()
}

// BuildCompileContext returns the values of the requested keys. It reads the
// values once and skips the keys that have no value. It never fails.
func (s *SiteContextService) BuildCompileContext(ctx context.Context, keys []string) (map[string]any, error) {
	values := map[string]any{}

	current := s.Values()
	for _, key := range keys {
		value, ok := current[key]
		if !ok {
			continue
		}
		values[key] = value
	}

	return values, nil
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
