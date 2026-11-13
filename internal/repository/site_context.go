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

// GetByID returns the site context of id and whether it exists.
func (r *SiteContextRepository) GetByID(id uint) (models.SiteContext, bool, error) {
	var row models.SiteContext
	err := r.db.Model(&models.SiteContext{}).Where("id = ?", id).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return models.SiteContext{}, false, nil
	case err != nil:
		return models.SiteContext{}, false, err
	}
	return row, true, nil
}

// Create inserts the site context of key and source and returns it.
func (r *SiteContextRepository) Create(key, source string) (models.SiteContext, error) {
	row := models.SiteContext{
		Key:    key,
		Source: source,
	}
	if err := r.db.Create(&row).Error; err != nil {
		return models.SiteContext{}, err
	}
	return row, nil
}

// UpdateByID sets the key and the source of the site context of id and
// returns the row that is read back.
func (r *SiteContextRepository) UpdateByID(id uint, key, source string) (models.SiteContext, error) {
	result := r.db.Model(&models.SiteContext{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"key":    key,
			"source": source,
		})
	if result.Error != nil {
		return models.SiteContext{}, result.Error
	}

	row, ok, err := r.GetByID(id)
	if err != nil {
		return models.SiteContext{}, err
	}
	if !ok {
		return models.SiteContext{}, gorm.ErrRecordNotFound
	}
	return row, nil
}

// GetAll returns the site contexts ordered by key.
func (r *SiteContextRepository) GetAll() ([]models.SiteContext, error) {
	var all []models.SiteContext
	if err := r.db.Model(&models.SiteContext{}).Order("key ASC").Find(&all).Error; err != nil {
		return nil, err
	}
	return all, nil
}

// Exists reports whether a site context of key exists.
func (r *SiteContextRepository) Exists(key string) (bool, error) {
	var count int64
	err := r.db.Model(&models.SiteContext{}).Where("key = ?", key).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Delete deletes the site context of key. It reports whether a site context
// was deleted.
func (r *SiteContextRepository) Delete(key string) (bool, error) {
	result := r.db.Where("key = ?", key).Delete(&models.SiteContext{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// Rename sets the key of the site context of oldKey to newKey. It reports
// whether a site context was renamed.
func (r *SiteContextRepository) Rename(oldKey, newKey string) (bool, error) {
	result := r.db.Model(&models.SiteContext{}).
		Where("key = ?", oldKey).
		Update("key", newKey)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
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
