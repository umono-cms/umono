package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFaviconServiceStartsEmpty(t *testing.T) {
	db := newFaviconTestDB(t)
	svc := newFaviconTestService(t, db)

	if _, ok := svc.Current(); ok {
		t.Fatal("expected no favicon")
	}
}

func TestFaviconServiceLoadsTheFaviconAtStartup(t *testing.T) {
	db := newFaviconTestDB(t)
	pngBytes := []byte("\x89PNG\r\n\x1a\nfirst")
	pngSum := sha256.Sum256(pngBytes)
	expectedSHA := hex.EncodeToString(pngSum[:])

	if err := db.Create(&models.Favicon{
		ID:          1,
		Content:     pngBytes,
		ContentType: "image/png",
		SHA256:      expectedSHA,
	}).Error; err != nil {
		t.Fatalf("create favicon: %v", err)
	}

	svc := newFaviconTestService(t, db)
	current, ok := svc.Current()
	if !ok {
		t.Fatal("expected favicon")
	}
	if !bytes.Equal(current.Content, pngBytes) {
		t.Fatalf("unexpected favicon content: %q", current.Content)
	}
	if current.ContentType != "image/png" {
		t.Fatalf("unexpected favicon content type: %q", current.ContentType)
	}
	if current.SHA256 != expectedSHA {
		t.Fatalf("unexpected favicon SHA256: got %q want %q", current.SHA256, expectedSHA)
	}
}

func TestFaviconServiceSaveReplacesTheOneRow(t *testing.T) {
	db := newFaviconTestDB(t)
	svc := newFaviconTestService(t, db)
	pngBytes := []byte("\x89PNG\r\n\x1a\nfirst")
	icoBytes := []byte("\x00\x00\x01\x00second")
	icoSum := sha256.Sum256(icoBytes)
	expectedSHA := hex.EncodeToString(icoSum[:])

	if err := svc.Save(pngBytes, "image/png"); err != nil {
		t.Fatalf("save png favicon: %v", err)
	}
	if err := svc.Save(icoBytes, "image/x-icon"); err != nil {
		t.Fatalf("save ico favicon: %v", err)
	}

	current, ok := svc.Current()
	if !ok {
		t.Fatal("expected favicon")
	}
	if !bytes.Equal(current.Content, icoBytes) {
		t.Fatalf("unexpected favicon content: %q", current.Content)
	}
	if current.ContentType != "image/x-icon" {
		t.Fatalf("unexpected favicon content type: %q", current.ContentType)
	}
	if current.SHA256 != expectedSHA {
		t.Fatalf("unexpected favicon SHA256: got %q want %q", current.SHA256, expectedSHA)
	}

	var count int64
	if err := db.Model(&models.Favicon{}).Count(&count).Error; err != nil {
		t.Fatalf("count favicon rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected favicon row count: got %d want 1", count)
	}

	second := newFaviconTestService(t, db)
	secondCurrent, ok := second.Current()
	if !ok {
		t.Fatal("expected favicon in second service")
	}
	if !bytes.Equal(secondCurrent.Content, icoBytes) {
		t.Fatalf("unexpected second service favicon content: %q", secondCurrent.Content)
	}
	if secondCurrent.ContentType != "image/x-icon" {
		t.Fatalf("unexpected second service favicon content type: %q", secondCurrent.ContentType)
	}
	if secondCurrent.SHA256 != expectedSHA {
		t.Fatalf("unexpected second service favicon SHA256: got %q want %q", secondCurrent.SHA256, expectedSHA)
	}
}

func TestFaviconServiceServesFromMemory(t *testing.T) {
	db := newFaviconTestDB(t)
	svc := newFaviconTestService(t, db)
	pngBytes := []byte("\x89PNG\r\n\x1a\nfirst")

	if err := svc.Save(pngBytes, "image/png"); err != nil {
		t.Fatalf("save favicon: %v", err)
	}
	if err := db.Exec("DELETE FROM favicons").Error; err != nil {
		t.Fatalf("delete favicon row: %v", err)
	}

	current, ok := svc.Current()
	if !ok {
		t.Fatal("expected favicon")
	}
	if !bytes.Equal(current.Content, pngBytes) {
		t.Fatalf("unexpected favicon content: %q", current.Content)
	}
}

func TestFaviconServiceKeepsItsOwnCopy(t *testing.T) {
	db := newFaviconTestDB(t)
	svc := newFaviconTestService(t, db)
	pngBytes := []byte("\x89PNG\r\n\x1a\nfirst")
	content := append([]byte(nil), pngBytes...)

	if err := svc.Save(content, "image/png"); err != nil {
		t.Fatalf("save favicon: %v", err)
	}
	content[0] = 'x'

	current, ok := svc.Current()
	if !ok {
		t.Fatal("expected favicon")
	}
	if !bytes.Equal(current.Content, pngBytes) {
		t.Fatalf("unexpected favicon content: %q", current.Content)
	}
}

func TestFaviconServiceRemove(t *testing.T) {
	db := newFaviconTestDB(t)
	svc := newFaviconTestService(t, db)
	pngBytes := []byte("\x89PNG\r\n\x1a\nfirst")

	if err := svc.Remove(); err != nil {
		t.Fatalf("remove empty favicon: %v", err)
	}
	if err := svc.Save(pngBytes, "image/png"); err != nil {
		t.Fatalf("save favicon: %v", err)
	}
	if err := svc.Remove(); err != nil {
		t.Fatalf("remove favicon: %v", err)
	}

	if _, ok := svc.Current(); ok {
		t.Fatal("expected no favicon after removal")
	}

	var count int64
	if err := db.Model(&models.Favicon{}).Count(&count).Error; err != nil {
		t.Fatalf("count favicon rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("unexpected favicon row count: got %d want 0", count)
	}

	second := newFaviconTestService(t, db)
	if _, ok := second.Current(); ok {
		t.Fatal("expected no favicon in second service")
	}
}

func newFaviconTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Favicon{}); err != nil {
		t.Fatalf("migrate favicon: %v", err)
	}

	return db
}

func newFaviconTestService(t *testing.T, db *gorm.DB) *FaviconService {
	t.Helper()

	svc, err := NewFaviconService(repository.NewFaviconRepository(db))
	if err != nil {
		t.Fatalf("create favicon service: %v", err)
	}

	return svc
}
