package service

import (
	"context"
	"strings"
	"testing"

	"github.com/umono-cms/umono/internal/models"
	"gorm.io/gorm"
)

func createNotFound(t *testing.T, db *gorm.DB) models.SitePage {
	t.Helper()
	key := models.NotFoundSystemKey
	p := models.SitePage{Name: "", Slug: models.NotFoundSlug, Enabled: true, Content: "", SystemKey: &key}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create not found system page: %v", err)
	}
	return p
}

func TestSaveNotFoundPageWithABinding(t *testing.T) {
	sps, _, db := newRenderTestServices(t)
	rec := storeBlogContract(t, db)
	nf := createNotFound(t, db)

	if err := sps.SaveNotFoundPage("Lost", "{{ UMONO_LAYOUT title = \"Gone\" }}\n", &rec.ID, strp("post")); err != nil {
		t.Fatalf("SaveNotFoundPage err = %v, want nil", err)
	}

	var got models.SitePage
	if err := db.First(&got, nf.ID).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.ContractID == nil || *got.ContractID != rec.ID {
		t.Fatalf("got.ContractID = %v, want %d", got.ContractID, rec.ID)
	}
	if got.LayoutID == nil || *got.LayoutID != "post" {
		t.Fatalf("got.LayoutID = %v, want %q", got.LayoutID, "post")
	}
	if got.Name != "Lost" {
		t.Fatalf("got.Name = %q, want %q", got.Name, "Lost")
	}

	ctx := context.Background()
	r, err := sps.GetNotFoundPage(ctx, "Default", "# Default")
	if err != nil {
		t.Fatalf("GetNotFoundPage err = %v, want nil", err)
	}
	if !strings.Contains(r.Content, "Gone") {
		t.Fatalf("strings.Contains(r.Content, %q) = false, want true", "Gone")
	}
}

func TestSaveNotFoundPageChecksTheBinding(t *testing.T) {
	sps, _, db := newRenderTestServices(t)
	rec := storeBlogContract(t, db)
	nf := createNotFound(t, db)

	e := bindingErr(t, sps.SaveNotFoundPage("Lost", "# Not a layout call\n", &rec.ID, strp("post")))
	if e.Kind != BindingRoot {
		t.Fatalf("e.Kind = %q, want %q", e.Kind, BindingRoot)
	}

	var got models.SitePage
	if err := db.First(&got, nf.ID).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.ContractID != nil {
		t.Fatalf("got.ContractID = %v, want nil", got.ContractID)
	}
	if got.Name != "" {
		t.Fatalf("got.Name = %q, want %q", got.Name, "")
	}
}

func TestBoundNotFoundPageHasNoDefaultContent(t *testing.T) {
	sps, _, db := newRenderTestServices(t)
	rec := storeBlogContract(t, db)
	nf := createNotFound(t, db)

	if err := db.Model(&models.SitePage{}).Where("id = ?", nf.ID).Updates(map[string]any{"contract_id": rec.ID, "layout_id": "post"}).Error; err != nil {
		t.Fatalf("update not found page: %v", err)
	}

	ctx := context.Background()
	r, err := sps.GetNotFoundPage(ctx, "Default", "# Default")
	if err != nil {
		t.Fatalf("GetNotFoundPage err = %v, want nil", err)
	}
	if strings.Contains(r.Content, "Default") {
		t.Fatalf("strings.Contains(r.Content, %q) = true, want false", "Default")
	}
	if r.Name != "Default" {
		t.Fatalf("r.Name = %q, want %q", r.Name, "Default")
	}
}

func TestPreviewBound(t *testing.T) {
	sps, _, db := newRenderTestServices(t)
	rec := storeBlogContract(t, db)

	ctx := context.Background()

	out, _, err := sps.PreviewBound(ctx, 0, "{{ UMONO_LAYOUT title = \"Hello\" }}\n", &rec.ID, strp("post"))
	if err != nil {
		t.Fatalf("PreviewBound err = %v, want nil", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Fatalf("strings.Contains(out, %q) = false, want true", "Hello")
	}

	out, diagnostics, err := sps.PreviewBound(ctx, 0, "{{ UMONO_LAYOUT title = \"Hello\" }}\n", nil, nil)
	if err != nil {
		t.Fatalf("PreviewBound err = %v, want nil", err)
	}
	_ = out
	if len(diagnostics) == 0 {
		t.Fatalf("len(diagnostics) = 0, want > 0")
	}
}
