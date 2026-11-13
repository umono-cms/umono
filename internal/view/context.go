package view

import (
	"github.com/umono-cms/umono/internal/models"
)

// ContextEditorData is the data of the editor of a site context.
type ContextEditorData struct {
	ID             uint
	Key            string
	Source         string
	KeyErr         string
	SourceErr      string
	LastModifiedAt string
	I18n           any
}

// ContextEditor builds the data of the editor of the site context row. The
// last modified time is only of a saved row.
func ContextEditor(row models.SiteContext, keyErr, sourceErr string, translator any) ContextEditorData {
	data := ContextEditorData{
		ID:        row.ID,
		Key:       row.Key,
		Source:    row.Source,
		KeyErr:    keyErr,
		SourceErr: sourceErr,
		I18n:      translator,
	}

	if row.ID != 0 {
		data.LastModifiedAt = RelativeTimeWithTranslator(&row.UpdatedAt, translator)
	}

	return data
}

// ContextLi is one row of the list of the site contexts.
type ContextLi struct {
	ID       uint
	Key      string
	IsActive bool
}

// ContextUl builds the list of the site contexts. The row of activeID is the
// active one.
func ContextUl(rows []models.SiteContext, activeID uint) []ContextLi {
	ul := []ContextLi{}
	for _, row := range rows {
		ul = append(ul, ContextLi{
			ID:       row.ID,
			Key:      row.Key,
			IsActive: row.ID == activeID,
		})
	}
	return ul
}
