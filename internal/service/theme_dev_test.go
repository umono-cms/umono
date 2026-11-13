package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/theme"
)

// writeDevTheme writes a theme project (root) and its dev directory,
// themes/dev/dev/0.0.0/, under themesDir as the CLI creates them. built.toml
// is the package fixture's with the id dev/dev and the version 0.0.0;
// contract.toml, attributes.toml and assets are symlinks into root. built,
// when not nil, changes built.toml before it is written.
func writeDevTheme(t *testing.T, themesDir string, built func(string) string) (root string) {
	t.Helper()
	root = t.TempDir()

	contractPath := filepath.Join(root, ".dev", "contracts", "example", "blog@v1.0.toml")
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(contractPath), err)
	}
	contractBytes, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", "contract.toml"))
	if err != nil {
		t.Fatalf("read fixture contract.toml: %v", err)
	}
	if err := os.WriteFile(contractPath, contractBytes, 0o644); err != nil {
		t.Fatalf("write %s: %v", contractPath, err)
	}

	attributesPath := filepath.Join(root, "attributes.toml")
	attributesBytes, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", "attributes.toml"))
	if err != nil {
		t.Fatalf("read fixture attributes.toml: %v", err)
	}
	if err := os.WriteFile(attributesPath, attributesBytes, 0o644); err != nil {
		t.Fatalf("write %s: %v", attributesPath, err)
	}

	assetsDir := filepath.Join(root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", assetsDir, err)
	}
	cssPath := filepath.Join(assetsDir, "theme.css")
	if err := os.WriteFile(cssPath, themecore.FallbackSource(), 0o644); err != nil {
		t.Fatalf("write %s: %v", cssPath, err)
	}

	builtBytes, err := os.ReadFile(filepath.Join("..", "theme", "testdata", "package", "built.toml"))
	if err != nil {
		t.Fatalf("read fixture built.toml: %v", err)
	}
	b := string(builtBytes)
	b = strings.Replace(b, "acme/minimal", "dev/dev", 1)
	b = strings.Replace(b, "0.1.0", "0.0.0", 1)
	if built != nil {
		b = built(b)
	}

	dir := filepath.Join(themesDir, "dev", "dev", "0.0.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	builtPath := filepath.Join(dir, "built.toml")
	if err := os.WriteFile(builtPath, []byte(b), 0o644); err != nil {
		t.Fatalf("write %s: %v", builtPath, err)
	}

	contractLink := filepath.Join(dir, "contract.toml")
	if err := os.Symlink(filepath.Join(root, ".dev", "contracts", "example", "blog@v1.0.toml"), contractLink); err != nil {
		t.Skipf("symlink: %v", err)
	}
	attributesLink := filepath.Join(dir, "attributes.toml")
	if err := os.Symlink(filepath.Join(root, "attributes.toml"), attributesLink); err != nil {
		t.Skipf("symlink: %v", err)
	}
	assetsLink := filepath.Join(dir, "assets")
	if err := os.Symlink(filepath.Join(root, "assets"), assetsLink); err != nil {
		t.Skipf("symlink: %v", err)
	}

	return root
}

// devRef is the contract of the dev theme's fixture.
func devRef(t *testing.T) models.Contract {
	t.Helper()
	ref, err := models.ParseContractRef("example/blog@v1.0")
	if err != nil {
		t.Fatalf("parse contract ref: %v", err)
	}
	return ref
}

func TestLoadDevAddsTheContractRecordWithoutTheRegistry(t *testing.T) {
	ctx := context.Background()
	reg := &fakeRegistry{}
	svc, db := newThemeTestService(t, reg)

	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)
	if err := svc.LoadDev(ctx); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	if reg.calls != 0 {
		t.Fatalf("reg.calls = %d, want 0", reg.calls)
	}

	ref := devRef(t)
	record, ok := svc.contracts.Record(ref)
	if !ok {
		t.Fatalf("contracts.Record(devRef) ok = false, want true")
	}
	if record.Source != string(contractSource(t)) {
		t.Fatalf("record.Source = %q, want %q", record.Source, string(contractSource(t)))
	}

	l, ok := svc.Loaded(ref.Group())
	if !ok {
		t.Fatalf("Loaded(devRef.Group()) ok = false, want true")
	}
	if l.Path != theme.DevPath {
		t.Fatalf("l.Path = %q, want %q", l.Path, theme.DevPath)
	}
	if l.Release != theme.DevRelease {
		t.Fatalf("l.Release = %q, want %q", l.Release, theme.DevRelease)
	}
	if l.ContractID != record.ID {
		t.Fatalf("l.ContractID = %d, want %d", l.ContractID, record.ID)
	}

	active, err := repository.NewThemeRepository(db).GetActiveThemes()
	if err != nil {
		t.Fatalf("GetActiveThemes: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0", len(active))
	}
}

func TestLoadDevThemeAPIOutOfRange(t *testing.T) {
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, func(b string) string {
		return strings.Replace(b, "theme-api-version = 1", fmt.Sprintf("theme-api-version = %d", themecore.ThemeAPICurrent()+1), 1)
	})
	svc.EnableDev(root)

	if err := svc.LoadDev(context.Background()); !errors.Is(err, theme.ErrThemeAPI) {
		t.Fatalf("LoadDev error = %v, want theme.ErrThemeAPI", err)
	}

	ref := devRef(t)
	if _, ok := svc.Loaded(ref.Group()); ok {
		t.Fatalf("Loaded(devRef.Group()) ok = true, want false")
	}
	if _, ok := svc.contracts.Record(ref); ok {
		t.Fatalf("contracts.Record(devRef) ok = true, want false")
	}
}

func TestLoadDevWithoutDevMode(t *testing.T) {
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	writeDevTheme(t, svc.themesDir, nil)

	if err := svc.LoadDev(context.Background()); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	ref := devRef(t)
	if _, ok := svc.Loaded(ref.Group()); ok {
		t.Fatalf("Loaded(devRef.Group()) ok = true, want false")
	}
	if _, ok := svc.contracts.Record(ref); ok {
		t.Fatalf("contracts.Record(devRef) ok = true, want false")
	}
}

func TestLoadDevChangedContract(t *testing.T) {
	svc, db := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)

	if err := db.Create(&models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0, Source: "s", Checksum: "x"}).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}

	if err := svc.LoadDev(context.Background()); !errors.Is(err, ErrContractChanged) {
		t.Fatalf("LoadDev error = %v, want ErrContractChanged", err)
	}

	g := devRef(t).Group()
	b, ok := svc.Broken(g)
	if !ok {
		t.Fatalf("Broken(devRef.Group()) ok = false, want true")
	}
	if b.Path != theme.DevPath {
		t.Fatalf("b.Path = %q, want %q", b.Path, theme.DevPath)
	}
	if _, ok := svc.Loaded(g); ok {
		t.Fatalf("Loaded(devRef.Group()) ok = true, want false")
	}
}

func TestLoadDevNotCovered(t *testing.T) {
	svc, db := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)

	newer := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 5, Source: "s", Checksum: "x"}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}

	layout := "post"
	if err := db.Create(&models.SitePage{Name: "A", Slug: "a", Enabled: true, ContractID: &newer.ID, LayoutID: &layout}).Error; err != nil {
		t.Fatalf("create page: %v", err)
	}

	if err := svc.LoadDev(context.Background()); !errors.Is(err, ErrNotCovered) {
		t.Fatalf("LoadDev error = %v, want ErrNotCovered", err)
	}
	if _, ok := svc.Loaded(devRef(t).Group()); ok {
		t.Fatalf("Loaded(devRef.Group()) ok = true, want false")
	}
}

func TestDevGroupRefusesActivateDeactivateAndRetry(t *testing.T) {
	ctx := context.Background()
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)
	if err := svc.LoadDev(ctx); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	g := devRef(t).Group()
	if _, err := svc.Activate(ctx, "acme/minimal/0.1.0"); !errors.Is(err, ErrDevTheme) {
		t.Fatalf("Activate error = %v, want ErrDevTheme", err)
	}
	if err := svc.Deactivate(g); !errors.Is(err, ErrDevTheme) {
		t.Fatalf("Deactivate error = %v, want ErrDevTheme", err)
	}
	if err := svc.Retry(ctx, g); !errors.Is(err, ErrDevTheme) {
		t.Fatalf("Retry error = %v, want ErrDevTheme", err)
	}

	l, ok := svc.Loaded(g)
	if !ok {
		t.Fatalf("Loaded(devRef.Group()) ok = false, want true")
	}
	if l.Path != theme.DevPath {
		t.Fatalf("l.Path = %q, want %q", l.Path, theme.DevPath)
	}
}

func TestDevThemeWinsOverActiveThemes(t *testing.T) {
	ctx := context.Background()
	svc, db := newThemeTestService(t, &fakeRegistry{})

	if _, err := svc.Activate(ctx, "acme/minimal/0.1.0"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	root := writeDevTheme(t, svc.themesDir, nil)
	svc2, _ := restart(t, db, svc.themesDir)
	if err := svc2.LoadActive(ctx); err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	svc2.EnableDev(root)
	if err := svc2.LoadDev(ctx); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	l, ok := svc2.Loaded(devRef(t).Group())
	if !ok {
		t.Fatalf("Loaded(devRef.Group()) ok = false, want true")
	}
	if l.Path != theme.DevPath {
		t.Fatalf("l.Path = %q, want %q", l.Path, theme.DevPath)
	}

	active, err := repository.NewThemeRepository(db).GetActiveThemes()
	if err != nil {
		t.Fatalf("GetActiveThemes: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("len(active) = %d, want 1", len(active))
	}
	if active[0].Path != "acme/minimal/0.1.0" {
		t.Fatalf("active[0].Path = %q, want %q", active[0].Path, "acme/minimal/0.1.0")
	}
}

func TestActivateDevWithoutDevModeFails(t *testing.T) {
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	writeDevTheme(t, svc.themesDir, nil)

	if _, err := svc.Activate(context.Background(), theme.DevPath); err == nil {
		t.Fatalf("Activate error = nil, want non-nil")
	}
}

// loadedDev returns a service whose theme in development is loaded, and
// the theme project.
func loadedDev(t *testing.T) (svc *ThemeService, root string) {
	t.Helper()
	svc, _ = newThemeTestService(t, &fakeRegistry{})
	root = writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)
	if err := svc.LoadDev(context.Background()); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}
	return
}

// writeFile writes b to path, creating its directory.
func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDevAssetTransformsThemeCSSOnEveryRequest(t *testing.T) {
	svc, root := loadedDev(t)

	b, ok, err := svc.DevAsset("theme.css")
	if err != nil {
		t.Fatalf("DevAsset(theme.css) error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("DevAsset(theme.css) ok = false, want true")
	}
	if !bytes.Equal(b, themecore.FallbackCSS()) {
		t.Fatalf("DevAsset(theme.css) bytes = %q, want %q", b, themecore.FallbackCSS())
	}

	writeFile(t, filepath.Join(root, "assets", "theme.css"), []byte(":scope { color: rebeccapurple; }"))

	b, ok, err = svc.DevAsset("theme.css")
	if err != nil {
		t.Fatalf("DevAsset(theme.css) error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("DevAsset(theme.css) ok = false, want true")
	}
	if !bytes.Contains(b, []byte("rebeccapurple")) {
		t.Fatalf("DevAsset(theme.css) bytes = %q, want contains %q", b, "rebeccapurple")
	}
}

func TestDevAssetCSSError(t *testing.T) {
	svc, root := loadedDev(t)

	writeFile(t, filepath.Join(root, "assets", "theme.css"), []byte("html { color: red }"))

	_, ok, err := svc.DevAsset("theme.css")
	if err == nil {
		t.Fatalf("DevAsset(theme.css) error = nil, want non-nil")
	}
	if ok {
		t.Fatalf("DevAsset(theme.css) ok = true, want false")
	}
}

func TestDevAssetServesAFileAndHidesABadOne(t *testing.T) {
	svc, root := loadedDev(t)

	writeFile(t, filepath.Join(root, "assets", "fonts", "a.woff2"), []byte("wOF2 font"))
	writeFile(t, filepath.Join(root, "assets", "bad.png"), []byte("not a png"))

	b, ok, err := svc.DevAsset("fonts/a.woff2")
	if err != nil {
		t.Fatalf("DevAsset(fonts/a.woff2) error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("DevAsset(fonts/a.woff2) ok = false, want true")
	}
	if string(b) != "wOF2 font" {
		t.Fatalf("DevAsset(fonts/a.woff2) bytes = %q, want %q", b, "wOF2 font")
	}

	for _, name := range []string{"bad.png", "missing.png"} {
		_, ok, err := svc.DevAsset(name)
		if err != nil {
			t.Fatalf("DevAsset(%s) error = %v, want nil", name, err)
		}
		if ok {
			t.Fatalf("DevAsset(%s) ok = true, want false", name)
		}
	}
}

func TestDevAssetWithoutLoadedDevTheme(t *testing.T) {
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)

	_, ok, err := svc.DevAsset("theme.css")
	if err != nil {
		t.Fatalf("DevAsset(theme.css) error = %v, want nil", err)
	}
	if ok {
		t.Fatalf("DevAsset(theme.css) ok = true, want false")
	}
}

func TestPageRenderCompilesDevAttributesOnEveryRequest(t *testing.T) {
	svc, root := loadedDev(t)

	record, ok := svc.contracts.Record(devRef(t))
	if !ok {
		t.Fatalf("contracts.Record(devRef) ok = false, want true")
	}
	layout := "post"
	page := models.SitePage{ContractID: &record.ID, LayoutID: &layout}

	r, err := svc.PageRender(page)
	if err != nil {
		t.Fatalf("PageRender error = %v, want nil", err)
	}
	if r.Release != theme.DevRelease {
		t.Fatalf("r.Release = %q, want %q", r.Release, theme.DevRelease)
	}
	if r.RootClass() != "layout-post" {
		t.Fatalf("r.RootClass() = %q, want %q", r.RootClass(), "layout-post")
	}

	b, err := os.ReadFile(filepath.Join(root, "attributes.toml"))
	if err != nil {
		t.Fatalf("read attributes.toml: %v", err)
	}
	writeFile(t, filepath.Join(root, "attributes.toml"), []byte(strings.Replace(string(b), "layout-post", "layout-changed", 1)))

	r, err = svc.PageRender(page)
	if err != nil {
		t.Fatalf("PageRender error = %v, want nil", err)
	}
	if r.RootClass() != "layout-changed" {
		t.Fatalf("r.RootClass() = %q, want %q", r.RootClass(), "layout-changed")
	}
}

func TestPageRenderFallsBackOnAFatalDevError(t *testing.T) {
	svc, _ := loadedDev(t)

	record, ok := svc.contracts.Record(devRef(t))
	if !ok {
		t.Fatalf("contracts.Record(devRef) ok = false, want true")
	}

	built := filepath.Join(svc.themesDir, "dev", "dev", "0.0.0", "built.toml")
	b, err := os.ReadFile(built)
	if err != nil {
		t.Fatalf("read %s: %v", built, err)
	}
	writeFile(t, built, []byte(strings.Replace(string(b), "theme-api-version = 1", fmt.Sprintf("theme-api-version = %d", themecore.ThemeAPICurrent()+1), 1)))

	layout := "post"
	r, err := svc.PageRender(models.SitePage{ContractID: &record.ID, LayoutID: &layout})
	if err != nil {
		t.Fatalf("PageRender error = %v, want nil", err)
	}
	if r.Release != "" {
		t.Fatalf("r.Release = %q, want %q", r.Release, "")
	}
	if r.Attributes != nil {
		t.Fatalf("r.Attributes = %v, want nil", r.Attributes)
	}
	if r.LayoutID != "post" {
		t.Fatalf("r.LayoutID = %q, want %q", r.LayoutID, "post")
	}
}

// listGroup returns the group g of the list, failing the test without one.
func listGroup(t *testing.T, list ThemeList, g models.ContractGroup) ThemeGroup {
	t.Helper()
	for _, x := range list.Groups {
		if x.Group == g {
			return x
		}
	}
	t.Fatalf("group %s not listed", g)
	return ThemeGroup{}
}

func TestListShowsTheThemeInDevelopment(t *testing.T) {
	svc, _ := loadedDev(t)

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Dev == nil {
		t.Fatalf("list.Dev = nil, want non-nil")
	}
	if list.Dev.Err != nil {
		t.Fatalf("list.Dev.Err = %v, want nil", list.Dev.Err)
	}
	if list.Dev.Built.Name != "Minimal" {
		t.Fatalf("list.Dev.Built.Name = %q, want %q", list.Dev.Built.Name, "Minimal")
	}
	if !list.Dev.HasGroup {
		t.Fatalf("list.Dev.HasGroup = false, want true")
	}
	if list.Dev.Group != devRef(t).Group() {
		t.Fatalf("list.Dev.Group = %s, want %s", list.Dev.Group, devRef(t).Group())
	}
	if len(list.Unreadable) != 0 {
		t.Fatalf("len(list.Unreadable) = %d, want 0", len(list.Unreadable))
	}
	if g := listGroup(t, list, devRef(t).Group()); !g.Dev {
		t.Fatalf("listGroup(list, devRef.Group()).Dev = false, want true")
	}
}

func TestListShowsTheFatalErrorOfTheThemeInDevelopment(t *testing.T) {
	svc, _ := loadedDev(t)

	built := filepath.Join(svc.themesDir, "dev", "dev", "0.0.0", "built.toml")
	b, err := os.ReadFile(built)
	if err != nil {
		t.Fatalf("read %s: %v", built, err)
	}
	writeFile(t, built, []byte(strings.Replace(string(b), "theme-api-version = 1", fmt.Sprintf("theme-api-version = %d", themecore.ThemeAPICurrent()+1), 1)))

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Dev == nil {
		t.Fatalf("list.Dev = nil, want non-nil")
	}
	if !errors.Is(list.Dev.Err, theme.ErrThemeAPI) {
		t.Fatalf("list.Dev.Err = %v, want theme.ErrThemeAPI", list.Dev.Err)
	}
}

func TestListShowsTheLoadingFailureOfTheThemeInDevelopment(t *testing.T) {
	svc, db := newThemeTestService(t, &fakeRegistry{})
	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)

	if err := db.Create(&models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0, Source: "s", Checksum: "x"}).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}

	if err := svc.LoadDev(context.Background()); !errors.Is(err, ErrContractChanged) {
		t.Fatalf("LoadDev error = %v, want ErrContractChanged", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Dev == nil {
		t.Fatalf("list.Dev = nil, want non-nil")
	}
	if !errors.Is(list.Dev.Err, ErrContractChanged) {
		t.Fatalf("list.Dev.Err = %v, want ErrContractChanged", list.Dev.Err)
	}
	if list.Dev.Built.Name != "Minimal" {
		t.Fatalf("list.Dev.Built.Name = %q, want %q", list.Dev.Built.Name, "Minimal")
	}
}

func TestListWithoutDevModeListsDevAsUnreadable(t *testing.T) {
	svc, _ := newThemeTestService(t, &fakeRegistry{})
	writeDevTheme(t, svc.themesDir, nil)

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Dev != nil {
		t.Fatalf("list.Dev = %v, want nil", list.Dev)
	}
	if len(list.Unreadable) != 1 {
		t.Fatalf("len(list.Unreadable) = %d, want 1", len(list.Unreadable))
	}
	if list.Unreadable[0].Path != theme.DevPath {
		t.Fatalf("list.Unreadable[0].Path = %q, want %q", list.Unreadable[0].Path, theme.DevPath)
	}
}

func TestListDevGroupIgnoresActiveThemes(t *testing.T) {
	ctx := context.Background()
	svc, db := newThemeTestService(t, &fakeRegistry{})

	if _, err := svc.Activate(ctx, "acme/minimal/0.1.0"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	root := writeDevTheme(t, svc.themesDir, nil)
	svc2, _ := restart(t, db, svc.themesDir)
	if err := svc2.LoadActive(ctx); err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	svc2.EnableDev(root)
	if err := svc2.LoadDev(ctx); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	list, err := svc2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	g := listGroup(t, list, devRef(t).Group())
	if !g.Dev {
		t.Fatalf("g.Dev = false, want true")
	}
	if g.Active != nil {
		t.Fatalf("g.Active = %v, want nil", g.Active)
	}
	if len(g.Themes) != 1 {
		t.Fatalf("len(g.Themes) = %d, want 1", len(g.Themes))
	}
	if g.Themes[0].Path != "acme/minimal/0.1.0" {
		t.Fatalf("g.Themes[0].Path = %q, want %q", g.Themes[0].Path, "acme/minimal/0.1.0")
	}
	if g.Themes[0].Active {
		t.Fatalf("g.Themes[0].Active = true, want false")
	}
}

func TestDeleteContractInDevMode(t *testing.T) {
	svc, db := newThemeTestService(t, &fakeRegistry{})

	// acme/minimal has the same contract; only the theme in development
	// may keep its record.
	if err := os.RemoveAll(filepath.Join(svc.themesDir, "acme")); err != nil {
		t.Fatalf("remove acme: %v", err)
	}

	root := writeDevTheme(t, svc.themesDir, nil)
	svc.EnableDev(root)
	if err := svc.LoadDev(context.Background()); err != nil {
		t.Fatalf("LoadDev: %v", err)
	}

	other := models.Contract{Org: "umono", Name: "page", Major: 0, Minor: 1, Source: "s", Checksum: "x"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other contract: %v", err)
	}

	dev := devRef(t)
	if err := svc.DeleteContract(dev); !errors.Is(err, ErrContractHasThemes) {
		t.Fatalf("DeleteContract(dev) error = %v, want ErrContractHasThemes", err)
	}
	if err := svc.DeleteContract(other); err != nil {
		t.Fatalf("DeleteContract(other): %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	g := listGroup(t, list, dev.Group())
	wantRef := dev.Ref()
	found := false
	for _, r := range g.Records {
		if r.Contract.Ref() == wantRef {
			if r.CanDelete {
				t.Fatalf("record CanDelete = true, want false")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("record with ref %s not in group %s", wantRef, dev.Group())
	}
}
