//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// missingPrefix starts the text of a translation key that has no text.
const missingPrefix = "[[missing: "

// setLanguage saves the admin language on the general settings screen.
func (p *adminPage) setLanguage(code, label string) {
	p.t.Helper()
	if _, err := p.Goto("/admin/settings/general"); err != nil {
		p.t.Fatalf("goto /admin/settings/general: %v", err)
	}
	if _, err := p.Locator("#language").SelectOption(playwright.SelectOptionValues{Values: &[]string{code}}); err != nil {
		p.t.Fatalf("select language %s: %v", code, err)
	}
	if err := p.Locator("#settings-general-save-button").Click(); err != nil {
		p.t.Fatalf("click settings-general-save-button: %v", err)
	}
	if err := expect.Locator(p.Locator(`label[for="language"]`)).ToHaveText(label); err != nil {
		p.t.Fatalf("assert language label %q: %v", label, err)
	}
}

func TestTurkishHasNoMissingTexts(t *testing.T) {
	p := newPage(t)
	p.login()
	p.setLanguage("tr", "Dil")
	t.Cleanup(func() { p.setLanguage("en", "Language") })

	paths := []string{
		"/admin/site-pages/new",
		"/admin/components/new",
		"/admin/settings/general",
		"/admin/settings/404-page",
		"/admin/settings/themes",
		"/admin/settings/robots-txt",
		"/admin/settings/storage",
		"/admin/settings/storage/new",
		"/admin/settings/about",
		"/admin/system/jobs",
		"/admin/system/errors",
		"/admin/context/new",
		"/admin/media",
		"/admin/media/new",
	}
	for _, path := range paths {
		if _, err := p.Goto(path); err != nil {
			t.Fatalf("goto %s: %v", path, err)
		}
		html, err := p.Content()
		if err != nil {
			t.Fatalf("read content of %s: %v", path, err)
		}
		if idx := strings.Index(html, missingPrefix); idx >= 0 {
			end := idx + 80
			if end > len(html) {
				end = len(html)
			}
			t.Errorf("%s has a text without translation: %s", path, html[idx:end])
		}
	}

	q := newPage(t)
	if _, err := q.Goto("/admin/login"); err != nil {
		t.Fatalf("goto /admin/login: %v", err)
	}
	html, err := q.Content()
	if err != nil {
		t.Fatalf("read content of /admin/login: %v", err)
	}
	if idx := strings.Index(html, missingPrefix); idx >= 0 {
		end := idx + 80
		if end > len(html) {
			end = len(html)
		}
		t.Errorf("/admin/login has a text without translation: %s", html[idx:end])
	}
}
