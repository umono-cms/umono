package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"

	"github.com/umono-cms/umono/internal/favicon"
	"github.com/umono-cms/umono/internal/repository"
)

// Favicon is the favicon of the site as it is served.
type Favicon struct {
	// Content is the bytes of the favicon.
	Content []byte
	// ContentType is the type of Content, image/png or image/x-icon.
	ContentType string
	// SHA256 is the lowercase hex SHA-256 of Content.
	SHA256 string
}

// FaviconService keeps the favicon in memory. It is loaded at startup and
// swapped on every save, so serving the favicon does not read the database.
// current is nil when there is no favicon.
type FaviconService struct {
	// writeMu serializes changes to the favicon. Reads do not take it.
	writeMu sync.Mutex
	// current points to the favicon that is served. It is nil when there is
	// no favicon. A change swaps it in.
	current atomic.Pointer[Favicon]

	repo *repository.FaviconRepository
}

// NewFaviconService loads the favicon of the site and keeps it in memory.
func NewFaviconService(repo *repository.FaviconRepository) (*FaviconService, error) {
	row, ok, err := repo.Get()
	if err != nil {
		return nil, err
	}

	s := &FaviconService{repo: repo}
	if ok {
		s.current.Store(&Favicon{
			Content:     row.Content,
			ContentType: row.ContentType,
			SHA256:      row.SHA256,
		})
	}

	return s, nil
}

// Current returns the favicon that is served and whether there is one. The
// returned Content must not be changed.
func (s *FaviconService) Current() (Favicon, bool) {
	current := s.current.Load()
	if current == nil {
		return Favicon{}, false
	}

	return *current, true
}

// Save saves the favicon of the site and swaps it in. It keeps a copy of the
// content, so a change to the content of the caller does not change the
// favicon that is served. When the save fails, the favicon in memory stays
// as it was. It does not check the type or the size of the content; Upload does.
func (s *FaviconService) Save(content []byte, contentType string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	cloned := bytes.Clone(content)
	sum := sha256.Sum256(cloned)
	sha := hex.EncodeToString(sum[:])

	if err := s.repo.Save(cloned, contentType, sha); err != nil {
		return err
	}

	s.current.Store(&Favicon{
		Content:     cloned,
		ContentType: contentType,
		SHA256:      sha,
	})

	return nil
}

// Upload saves content as the favicon of the site with the type detected from
// its bytes. A content that favicon.Detect rejects is not saved and the
// favicon stays as it was; the error wraps favicon.ErrTooLarge or
// favicon.ErrFormat.
func (s *FaviconService) Upload(content []byte) error {
	contentType, err := favicon.Detect(content)
	if err != nil {
		return err
	}

	return s.Save(content, contentType)
}

// Remove removes the favicon of the site and clears the one in memory. When
// the removal fails, the favicon in memory stays as it was.
func (s *FaviconService) Remove() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.Delete(); err != nil {
		return err
	}

	s.current.Store(nil)

	return nil
}
