package repository

import (
	"errors"

	"github.com/umono-cms/umono/internal/models"
	"gorm.io/gorm"
)

type SiteContextRepository struct {
	db *gorm.DB
}

func NewSiteContextRepository(db *gorm.DB) *SiteContextRepository {
	return &SiteContextRepository{
		db: db,
	}
}

// GetAll returns the site contexts ordered by key.
func (r *SiteContextRepository) GetAll() ([]models.SiteContext, error) {
	var all []models.SiteContext
	if err := r.db.Model(&models.SiteContext{}).Order("key ASC").Find(&all).Error; err != nil {
		return nil, err
	}
	return all, nil
}

// Save creates the site context of key, or updates the source of the one the
// key has.
func (r *SiteContextRepository) Save(key, source string) error {
	var existing models.SiteContext
	err := r.db.Model(&models.SiteContext{}).Where("key = ?", key).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return r.db.Create(&models.SiteContext{
			Key:    key,
			Source: source,
		}).Error
	case err != nil:
		return err
	}

	return r.db.Model(&models.SiteContext{}).
		Where("key = ?", key).
		Update("source", source).
		Error
}
