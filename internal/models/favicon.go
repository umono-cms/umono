package models

import "time"

// FaviconID is the ID of the one row the favicon table holds.
const FaviconID uint = 1

// Favicon is the icon the site owner uploads. The table has at most one row,
// the row of FaviconID.
type Favicon struct {
	// ID is the primary key of the row.
	ID uint `gorm:"primaryKey"`
	// Content is the file as uploaded.
	Content []byte `gorm:"not null"`
	// ContentType is the type detected from the bytes of Content
	// (image/png or image/x-icon).
	ContentType string `gorm:"not null"`
	// SHA256 is the lowercase hex SHA-256 of Content.
	SHA256 string `gorm:"not null"`
	// UpdatedAt is the time the row was written last.
	UpdatedAt time.Time
}
