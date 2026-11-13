package service

import (
	"bytes"
	"context"
	"testing"

	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestComponentServiceNotifiesRenderInputChange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Component{}); err != nil {
		t.Fatalf("migrate component: %v", err)
	}

	cc, err := NewContentCompiler(nil)
	if err != nil {
		t.Fatalf("new content compiler: %v", err)
	}

	svc := NewComponentService(repository.NewComponentRepository(db))
	svc.SetContentCompiler(cc)

	calls := 0
	svc.OnRenderInputChange(func(context.Context) {
		calls++
	})

	if _, errs := svc.Create(models.Component{Name: "bad name", Content: "x"}); len(errs) == 0 {
		t.Fatalf("Create() errors = %v, want non-empty", errs)
	}
	if got, want := calls, 0; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	created, errs := svc.Create(models.Component{Name: "FOO", Content: "Foo"})
	if len(errs) != 0 {
		t.Fatalf("Create() errors = %v, want empty", errs)
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	created.Content = "Bar"
	if _, errs := svc.Update(created); len(errs) != 0 {
		t.Fatalf("Update() errors = %v, want empty", errs)
	}
	if got, want := calls, 2; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete() err = %v, want nil", err)
	}
	if got, want := calls, 3; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}
}

func TestMediaServiceNotifiesRenderInputChange(t *testing.T) {
	db := newMediaVariantTestDB(t)
	mediaSvc := NewMediaService(repository.NewMediaRepository(db), repository.NewStorageRepository(db), nil, nil, t.TempDir())
	if err := mediaSvc.EnsureDefaultLocalStorage(t.TempDir()); err != nil {
		t.Fatalf("ensure local storage: %v", err)
	}

	calls := 0
	mediaSvc.OnRenderInputChange(func(context.Context) {
		calls++
	})

	result, err := mediaSvc.Upload(context.Background(), UploadMediaInput{
		StorageID:    DefaultLocalStorageID,
		OriginalName: "hero.png",
		MimeType:     "image/png",
		Reader:       bytes.NewReader(testPNG(t, 40, 20)),
	})
	if err != nil {
		t.Fatalf("Upload() err = %v, want nil", err)
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if _, err := mediaSvc.UpdateAlias(result.Media.ID, "Not Valid"); err == nil {
		t.Fatalf("UpdateAlias() err = %v, want non-nil", err)
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if _, err := mediaSvc.UpdateAlias(result.Media.ID, "hero"); err != nil {
		t.Fatalf("UpdateAlias() err = %v, want nil", err)
	}
	if got, want := calls, 2; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := mediaSvc.Delete(context.Background(), result.Media.ID); err != nil {
		t.Fatalf("Delete() err = %v, want nil", err)
	}
	if got, want := calls, 3; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}
}

func TestSiteContextServiceNotifiesRenderInputChange(t *testing.T) {
	db := newSiteContextTestDB(t)

	svc, err := NewSiteContextService(repository.NewSiteContextRepository(db))
	if err != nil {
		t.Fatalf("NewSiteContextService() error = %v", err)
	}

	calls := 0
	svc.OnRenderInputChange(func(context.Context) {
		calls++
	})

	if _, err := svc.Save("site/count", "[1,"); err == nil {
		t.Fatalf("Save() error = %v, want non-nil", err)
	}
	if got, want := calls, 0; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if _, err := svc.Save("bad", "1"); err == nil {
		t.Fatalf("Save() error = %v, want non-nil", err)
	}
	if got, want := calls, 0; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if _, err := svc.Save("site/count", "1"); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if got, want := calls, 1; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if _, err := svc.Save("site/count", "2"); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if got, want := calls, 2; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := svc.Rename("site/count", "site/total"); err != nil {
		t.Fatalf("Rename() error = %v, want nil", err)
	}
	if got, want := calls, 3; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := svc.Rename("site/missing", "site/x"); err == nil {
		t.Fatalf("Rename() error = %v, want non-nil", err)
	}
	if got, want := calls, 3; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := svc.Delete("site/total"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if got, want := calls, 4; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}

	if err := svc.Delete("site/total"); err == nil {
		t.Fatalf("Delete() error = %v, want non-nil", err)
	}
	if got, want := calls, 4; got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}
}
