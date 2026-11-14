package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/theme"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeRegistry struct {
	calls int
	ref   models.Contract
	src   []byte
	err   error
	host  string
}

func (f *fakeRegistry) Verify(ctx context.Context, ref models.Contract, src []byte) error {
	f.calls++
	f.ref = ref
	f.src = src
	return f.err
}

func (f *fakeRegistry) Host() string {
	return f.host
}

func contractSource(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", "contract.toml"))
	if err != nil {
		t.Fatalf("read contract fixture: %v", err)
	}
	return b
}

func writeTestTheme(t *testing.T) string {
	t.Helper()
	return writeTestThemeWith(t, nil)
}

func writeTestThemeWith(t *testing.T, mutate func(files map[string][]byte)) string {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range []string{"built.toml", "contract.toml", "attributes.toml", "LICENSE"} {
		b, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		files[name] = b
	}
	files["assets/theme.css"] = themecore.FallbackCSS()
	if mutate != nil {
		mutate(files)
	}
	sum, err := themecore.Checksums(themecore.Package{Files: files})
	if err != nil {
		t.Fatalf("checksums: %v", err)
	}
	files["checksums.json"] = sum
	dir := t.TempDir()
	for name, b := range files {
		full := filepath.Join(dir, "acme", "minimal", "0.1.0", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

func newThemeTestService(t *testing.T, reg *fakeRegistry) (*ThemeService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Contract{}, &models.SitePage{}, &models.Option{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewThemeService(writeTestTheme(t), t.TempDir(), NewContractService(repository.NewContractRepository(db)), repository.NewThemeRepository(db), reg), db
}

func TestThemeServiceCheckAsksTheRegistryForANewContract(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, _ := newThemeTestService(t, reg)

	c, err := svc.Check(ctx, "acme/minimal/0.1.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if reg.calls != 1 {
		t.Fatalf("reg.calls = %d, want 1", reg.calls)
	}
	if reg.ref.Org != "example" {
		t.Fatalf("reg.ref.Org = %q, want %q", reg.ref.Org, "example")
	}
	if reg.ref.Name != "blog" {
		t.Fatalf("reg.ref.Name = %q, want %q", reg.ref.Name, "blog")
	}
	if reg.ref.Major != 1 {
		t.Fatalf("reg.ref.Major = %d, want 1", reg.ref.Major)
	}
	if reg.ref.Minor != 0 {
		t.Fatalf("reg.ref.Minor = %d, want 0", reg.ref.Minor)
	}
	src := contractSource(t)
	if !bytes.Equal(reg.src, src) {
		t.Fatalf("reg.src = %q, want %q", reg.src, src)
	}
	if !c.Asked {
		t.Fatalf("c.Asked = false, want true")
	}
	if c.Path != "acme/minimal/0.1.0" {
		t.Fatalf("c.Path = %q, want %q", c.Path, "acme/minimal/0.1.0")
	}
	if c.Release == "" {
		t.Fatalf("c.Release is empty")
	}
	if c.Contract.ID != 0 {
		t.Fatalf("c.Contract.ID = %d, want 0", c.Contract.ID)
	}
	if c.Contract.Org != "example" {
		t.Fatalf("c.Contract.Org = %q, want %q", c.Contract.Org, "example")
	}
	if c.Contract.Name != "blog" {
		t.Fatalf("c.Contract.Name = %q, want %q", c.Contract.Name, "blog")
	}
	if c.Contract.Major != 1 {
		t.Fatalf("c.Contract.Major = %d, want 1", c.Contract.Major)
	}
	if c.Contract.Minor != 0 {
		t.Fatalf("c.Contract.Minor = %d, want 0", c.Contract.Minor)
	}
	if c.Contract.Source != string(src) {
		t.Fatalf("c.Contract.Source = %q, want %q", c.Contract.Source, string(src))
	}
	sum := sha256.Sum256(src)
	if c.Contract.Checksum != hex.EncodeToString(sum[:]) {
		t.Fatalf("c.Contract.Checksum = %q, want %q", c.Contract.Checksum, hex.EncodeToString(sum[:]))
	}
}

func TestThemeServiceCheckUsesTheStoredContract(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, db := newThemeTestService(t, reg)

	src := contractSource(t)
	sum := sha256.Sum256(src)
	rec := models.Contract{
		Org:      "example",
		Name:     "blog",
		Major:    1,
		Minor:    0,
		Source:   string(src),
		Checksum: hex.EncodeToString(sum[:]),
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}

	c, err := svc.Check(ctx, "acme/minimal/0.1.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if reg.calls != 0 {
		t.Fatalf("reg.calls = %d, want 0", reg.calls)
	}
	if c.Asked {
		t.Fatalf("c.Asked = true, want false")
	}
	if c.Contract.ID != rec.ID {
		t.Fatalf("c.Contract.ID = %d, want %d", c.Contract.ID, rec.ID)
	}
}

func TestThemeServiceCheckRejectsAChangedContract(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, db := newThemeTestService(t, reg)

	src := contractSource(t)
	rec := models.Contract{
		Org:      "example",
		Name:     "blog",
		Major:    1,
		Minor:    0,
		Source:   string(src),
		Checksum: "0000",
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}

	_, err := svc.Check(ctx, "acme/minimal/0.1.0")
	if !errors.Is(err, ErrContractChanged) {
		t.Fatalf("Check error = %v, want ErrContractChanged", err)
	}
	if reg.calls != 0 {
		t.Fatalf("reg.calls = %d, want 0", reg.calls)
	}
}

func TestThemeServiceCheckReturnsTheRegistryError(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{err: theme.ErrContractNotPublished}
	svc, _ := newThemeTestService(t, reg)

	_, err := svc.Check(ctx, "acme/minimal/0.1.0")
	if !errors.Is(err, theme.ErrContractNotPublished) {
		t.Fatalf("Check error = %v, want theme.ErrContractNotPublished", err)
	}
}

func TestThemeServiceCheckRejectsAnUncoveredBase(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, db := newThemeTestService(t, reg)

	rec := models.Contract{
		Org:      "example",
		Name:     "blog",
		Major:    1,
		Minor:    1,
		Source:   "s",
		Checksum: "x",
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}
	layout := "post"
	page := models.SitePage{
		Name:       "P",
		Slug:       "p",
		Enabled:    true,
		ContractID: &rec.ID,
		LayoutID:   &layout,
	}
	if err := db.Create(&page).Error; err != nil {
		t.Fatalf("create page: %v", err)
	}

	_, err := svc.Check(ctx, "acme/minimal/0.1.0")
	if !errors.Is(err, ErrNotCovered) {
		t.Fatalf("Check error = %v, want ErrNotCovered", err)
	}
}

func TestThemeServiceCheckRunsThePackageChecksFirst(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, _ := newThemeTestService(t, reg)

	_, err := svc.Check(ctx, "../acme/minimal/0.1.0")
	if !errors.Is(err, theme.ErrInvalidThemePath) {
		t.Fatalf("Check error = %v, want theme.ErrInvalidThemePath", err)
	}
	if reg.calls != 0 {
		t.Fatalf("reg.calls = %d, want 0", reg.calls)
	}
}

func TestThemeServiceRegistryHost(t *testing.T) {
	reg := &fakeRegistry{host: "registry.example.lan"}
	svc, _ := newThemeTestService(t, reg)

	if got, want := svc.RegistryHost(), "registry.example.lan"; got != want {
		t.Fatalf("RegistryHost() = %q, want %q", got, want)
	}
}
