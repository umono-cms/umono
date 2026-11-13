package models

import "time"

// SiteContext is a named value the site owner defines. Source is the value
// as written.
type SiteContext struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"uniqueIndex;not null"`
	Source    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
