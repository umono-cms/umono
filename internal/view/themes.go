package view

import (
	"fmt"
	"time"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/service"
	"github.com/umono-cms/umono/internal/theme"
)

// ThemesPage is the theme management page.
type ThemesPage struct {
	Groups     []ThemeGroupView
	Unreadable []UnreadableThemeView
	// Pending are the pending installs without an installed theme at their
	// path. They can only be rejected. The others are shown on their theme.
	Pending []PendingInstallView
	// Dev is the theme in development in dev mode, nil without the dev mode.
	Dev             *DevThemeView
	ThemeAPIMin     int
	ThemeAPICurrent int
}

// ThemeGroupView is a contract group of the page.
type ThemeGroupView struct {
	// Key is the group key, for example "umono/e-commerce v1".
	Key       string
	PageCount int64
	// Base is the base version, for example "v1.3", "" without a base.
	Base   string
	Active *ActiveThemeView
	// Dev tells whether the group's active theme is the theme in
	// development. Its themes cannot be activated then.
	Dev     bool
	Themes  []ThemeView
	Records []RecordView
}

// ActiveThemeView is a group's active theme.
type ActiveThemeView struct {
	Path string
	// ContractID is org/id@vMAJOR.MINOR; the actions name the group with it.
	ContractID string
	// Broken is the reason the theme failed the checks of loading, "" when
	// it is loaded.
	Broken string
}

// DevThemeView is the theme in development, from the built.toml of the
// current check.
type DevThemeView struct {
	Name            string
	Contract        string
	ThemeAPIVersion int
	// Error is why the theme is not active, "" when it is.
	Error string
}

// ThemeView is an installed theme of a group.
type ThemeView struct {
	Path    string
	ID      string
	Name    string
	Version string
	// Contract is the reference of the theme's contract and ContractVersion
	// its version, for example "v1.3".
	Contract        string
	ContractVersion string
	Active          bool
	UncoveredPages  int
	ThemeAPI        bool
	ThemeAPIVersion int
	// Selectable is false for the active theme and for a theme the list
	// shows a reason for.
	Selectable bool
	// Registry tells whether activation asks the registry (umono.dev or the
	// mirror UMONO_REGISTRY names): the site has no stored record of the
	// contract.
	Registry bool
	Pending  *PendingInstallView
}

// PendingInstallView is a pending install of umono theme install --force.
type PendingInstallView struct {
	Path        string
	Installed   bool
	Active      bool
	OldBuiltAt  string
	NewBuiltAt  string
	Changed     []string
	Added       []string
	Removed     []string
	OldThemeAPI int
	NewThemeAPI int
	OldContract string
	NewContract string
	// Error is why a side could not be read; such an install can only be
	// rejected.
	Error string
}

// RecordView is a stored contract record of a group.
type RecordView struct {
	Ref       string
	Version   string
	PageCount int64
	CanDelete bool
}

// UnreadableThemeView is an installed theme whose built.toml cannot be read.
type UnreadableThemeView struct {
	Path  string
	Error string
}

// NewThemesPage builds the theme management page from the theme list.
func NewThemesPage(list service.ThemeList) ThemesPage {
	page := ThemesPage{ThemeAPIMin: themecore.ThemeAPIMin(), ThemeAPICurrent: themecore.ThemeAPICurrent()}

	installed := map[string]bool{}
	active := map[string]bool{}
	for _, g := range list.Groups {
		for _, t := range g.Themes {
			installed[t.Path] = true
			if t.Active {
				active[t.Path] = true
			}
		}
	}
	pending := map[string]*PendingInstallView{}
	for _, p := range list.Pending {
		v := pendingInstallView(p, active[p.Path])
		if p.Installed && installed[p.Path] {
			pending[p.Path] = &v
			continue
		}
		page.Pending = append(page.Pending, v)
	}

	for _, g := range list.Groups {
		gv := ThemeGroupView{Key: g.Group.String(), PageCount: g.PageCount, Dev: g.Dev}
		if g.HasBase {
			gv.Base = contractVersion(g.Group.Major, g.Base)
		}
		stored := map[string]bool{}
		for _, r := range g.Records {
			stored[r.Contract.Ref()] = true
			gv.Records = append(gv.Records, RecordView{
				Ref:       r.Contract.Ref(),
				Version:   contractVersion(r.Contract.Major, r.Contract.Minor),
				PageCount: r.PageCount,
				CanDelete: r.CanDelete,
			})
		}
		if g.Active != nil {
			av := &ActiveThemeView{Path: g.Active.Path, ContractID: g.Active.ContractID}
			if g.Active.Broken != nil && g.Active.Broken.Err != nil {
				av.Broken = g.Active.Broken.Err.Error()
			}
			gv.Active = av
		}
		for _, t := range g.Themes {
			gv.Themes = append(gv.Themes, ThemeView{
				Path:            t.Path,
				ID:              t.Built.ID,
				Name:            t.Built.Name,
				Version:         t.Built.Version,
				Contract:        t.Contract.Ref(),
				ContractVersion: contractVersion(t.Contract.Major, t.Contract.Minor),
				Active:          t.Active,
				UncoveredPages:  t.UncoveredPages,
				ThemeAPI:        t.ThemeAPI,
				ThemeAPIVersion: t.Built.ThemeAPIVersion,
				Selectable:      !t.Active && t.UncoveredPages == 0 && !t.ThemeAPI && !g.Dev,
				Registry:        !stored[t.Contract.Ref()],
				Pending:         pending[t.Path],
			})
		}
		page.Groups = append(page.Groups, gv)
	}

	if d := list.Dev; d != nil {
		page.Dev = &DevThemeView{Name: d.Built.Name, Contract: d.Built.Contract, ThemeAPIVersion: d.Built.ThemeAPIVersion}
		if d.Err != nil {
			page.Dev.Error = d.Err.Error()
		}
	}

	for _, u := range list.Unreadable {
		v := UnreadableThemeView{Path: u.Path}
		if u.Err != nil {
			v.Error = u.Err.Error()
		}
		page.Unreadable = append(page.Unreadable, v)
	}
	return page
}

func pendingInstallView(p theme.PendingInstall, active bool) PendingInstallView {
	v := PendingInstallView{
		Path:        p.Path,
		Installed:   p.Installed,
		Active:      active,
		Changed:     p.Changed,
		Added:       p.Added,
		Removed:     p.Removed,
		OldThemeAPI: p.OldBuilt.ThemeAPIVersion,
		NewThemeAPI: p.NewBuilt.ThemeAPIVersion,
		OldContract: p.OldBuilt.Contract,
		NewContract: p.NewBuilt.Contract,
		OldBuiltAt:  builtAt(p.OldBuilt.BuiltAt),
		NewBuiltAt:  builtAt(p.NewBuilt.BuiltAt),
	}
	if p.Err != nil {
		v.Error = p.Err.Error()
	}
	return v
}

func builtAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func contractVersion(major, minor uint) string {
	return fmt.Sprintf("v%d.%d", major, minor)
}

// ThemePagesView lists the pages that keep a theme of Contract from being
// selected: the pages of its group bound to a newer minor.
type ThemePagesView struct {
	// Contract is the reference of the theme's contract, for example
	// "example/blog@v1.0".
	Contract string
	Pages    []ThemePageView
}

// ThemePageView is a page bound to a newer minor. Record is the version of
// the record it is bound to, for example "v1.3".
type ThemePageView struct {
	ID     uint
	Name   string
	Slug   string
	Record string
}

// NewThemePagesView builds the view of the pages above the minor of ref.
func NewThemePagesView(ref models.Contract, pages []models.SitePage) ThemePagesView {
	v := ThemePagesView{Contract: ref.Ref()}
	for _, p := range pages {
		pv := ThemePageView{ID: p.ID, Name: p.Name, Slug: p.Slug}
		if p.Contract != nil {
			pv.Record = contractVersion(p.Contract.Major, p.Contract.Minor)
		}
		v.Pages = append(v.Pages, pv)
	}
	return v
}
