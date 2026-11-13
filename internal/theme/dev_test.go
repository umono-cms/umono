package theme

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umono-cms/themecore"
)

// writeDevTheme writes a theme project (root) and its dev directory under
// themesDir, themes/dev/dev/0.0.0/, as the CLI creates them. built.toml is
// the package fixture's with the id dev/dev and the version 0.0.0;
// contract.toml, attributes.toml and assets are symlinks into root. built,
// when not nil, changes built.toml before it is written.
func writeDevTheme(t *testing.T, built func(string) string) (themesDir, root string) {
	t.Helper()
	root = t.TempDir()

	contractPath := filepath.Join(root, ".dev", "contracts", "example", "blog@v1.0.toml")
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(contractPath), err)
	}
	contractBytes, err := os.ReadFile(filepath.Join("testdata", "package", "contract.toml"))
	if err != nil {
		t.Fatalf("read fixture contract.toml: %v", err)
	}
	if err := os.WriteFile(contractPath, contractBytes, 0o644); err != nil {
		t.Fatalf("write %s: %v", contractPath, err)
	}

	attributesPath := filepath.Join(root, "attributes.toml")
	attributesBytes, err := os.ReadFile(filepath.Join("testdata", "package", "attributes.toml"))
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

	builtBytes, err := os.ReadFile(filepath.Join("testdata", "package", "built.toml"))
	if err != nil {
		t.Fatalf("read fixture built.toml: %v", err)
	}
	b := string(builtBytes)
	b = strings.Replace(b, "acme/minimal", "dev/dev", 1)
	b = strings.Replace(b, "0.1.0", "0.0.0", 1)
	if built != nil {
		b = built(b)
	}

	themesDir = t.TempDir()
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

	return themesDir, root
}

func TestParseDevRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}

	inputs := []string{
		"",
		"themes",
		filepath.Join(t.TempDir(), "missing"),
		file,
	}
	for _, in := range inputs {
		in := in
		t.Run(in, func(t *testing.T) {
			if _, err := ParseDevRoot(in); !errors.Is(err, ErrDevRoot) {
				t.Fatalf("ParseDevRoot(%q): errors.Is(err, ErrDevRoot) = false; err = %v", in, err)
			}
		})
	}

	dir := t.TempDir()
	got, err := ParseDevRoot(dir)
	if err != nil {
		t.Fatalf("ParseDevRoot(%q): %v", dir, err)
	}
	if got != dir {
		t.Fatalf("ParseDevRoot(%q) = %q, want %q", dir, got, dir)
	}
}

func TestCheckDev(t *testing.T) {
	themesDir, root := writeDevTheme(t, nil)
	r, err := CheckDev(themesDir, root)
	if err != nil {
		t.Fatalf("CheckDev: %v", err)
	}
	if r.Built.ID != "dev/dev" {
		t.Fatalf("Built.ID = %q, want %q", r.Built.ID, "dev/dev")
	}
	if !bytes.Equal(r.CSS, themecore.FallbackCSS()) {
		t.Fatalf("CSS = %q, want %q", r.CSS, themecore.FallbackCSS())
	}
}

func TestCheckDevThemeAPIOutOfRange(t *testing.T) {
	themesDir, root := writeDevTheme(t, func(b string) string {
		return strings.Replace(b, "theme-api-version = 1", fmt.Sprintf("theme-api-version = %d", themecore.ThemeAPICurrent()+1), 1)
	})
	if _, err := CheckDev(themesDir, root); !errors.Is(err, ErrThemeAPI) {
		t.Fatalf("CheckDev: errors.Is(err, ErrThemeAPI) = false; err = %v", err)
	}
}

func TestCheckDevSymlinkOutsideRoot(t *testing.T) {
	themesDir, root := writeDevTheme(t, nil)

	outside := filepath.Join(t.TempDir(), "attributes.toml")
	attributesBytes, err := os.ReadFile(filepath.Join("testdata", "package", "attributes.toml"))
	if err != nil {
		t.Fatalf("read fixture attributes.toml: %v", err)
	}
	if err := os.WriteFile(outside, attributesBytes, 0o644); err != nil {
		t.Fatalf("write %s: %v", outside, err)
	}

	dir := filepath.Join(themesDir, "dev", "dev", "0.0.0")
	if err := os.Remove(filepath.Join(dir, "attributes.toml")); err != nil {
		t.Fatalf("remove attributes.toml: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "attributes.toml")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	if _, err := CheckDev(themesDir, root); err == nil {
		t.Fatalf("CheckDev: want error, got nil")
	}
}

func TestReadDevAsset(t *testing.T) {
	themesDir, root := writeDevTheme(t, nil)

	fontPath := filepath.Join(root, "assets", "fonts", "a.woff2")
	if err := os.MkdirAll(filepath.Dir(fontPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(fontPath), err)
	}
	if err := os.WriteFile(fontPath, []byte("wOF2 font"), 0o644); err != nil {
		t.Fatalf("write %s: %v", fontPath, err)
	}

	b, err := ReadDevAsset(themesDir, "fonts/a.woff2")
	if err != nil {
		t.Fatalf("ReadDevAsset(fonts/a.woff2): %v", err)
	}
	if string(b) != "wOF2 font" {
		t.Fatalf("ReadDevAsset(fonts/a.woff2) = %q, want %q", string(b), "wOF2 font")
	}

	if _, err := ReadDevAsset(themesDir, "../built.toml"); err == nil {
		t.Fatalf("ReadDevAsset(../built.toml): want error, got nil")
	}

	if _, err := ReadDevAsset(themesDir, "missing.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadDevAsset(missing.png): errors.Is(err, fs.ErrNotExist) = false; err = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "out.png")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", outside, err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "assets", "out.png")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if _, err := ReadDevAsset(themesDir, "out.png"); err == nil {
		t.Fatalf("ReadDevAsset(out.png): want error, got nil")
	}
}
