package repository

import (
	"errors"

	"github.com/umono-cms/umono/internal/models"
	"gorm.io/gorm"
)

// FaviconRepository reads and writes the favicon of the site.
type FaviconRepository struct {
	db *gorm.DB
}

// NewFaviconRepository returns a favicon repository that uses db.
func NewFaviconRepository(db *gorm.DB) *FaviconRepository {
	return &FaviconRepository{
		db: db,
	}
}

// Get returns the favicon of the site and whether it exists.
func (r *FaviconRepository) Get() (models.Favicon, bool, error) {
	var row models.Favicon
	err := r.db.Model(&models.Favicon{}).Where("id = ?", models.FaviconID).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return models.Favicon{}, false, nil
	case err != nil:
		return models.Favicon{}, false, err
	}
	return row, true, nil
}

// Save writes the favicon of the site, inserting the row of models.FaviconID
// or replacing its content, content type and hash.
func (r *FaviconRepository) Save(content []byte, contentType, sha256 string) error {
	return r.db.Save(&models.Favicon{
		ID:          models.FaviconID,
		Content:     content,
		ContentType: contentType,
		SHA256:      sha256,
	}).Error
}

// Delete deletes the favicon of the site. It is not an error when there is
// no favicon.
func (r *FaviconRepository) Delete() error {
	return r.db.Where("id = ?", models.FaviconID).Delete(&models.Favicon{}).Error
}
