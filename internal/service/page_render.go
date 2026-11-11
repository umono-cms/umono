package service

import (
	"errors"
	"fmt"

	"github.com/umono-cms/compono"
	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/theme"
)

// layoutGlobal is the name of the global that renders a page's layout.
const layoutGlobal = "UMONO_LAYOUT"

// ErrLayoutNotFound is returned when the contract record a page is bound to
// has no layout with the page's LayoutID.
var ErrLayoutNotFound = errors.New("the contract has no such layout")

// PageRender is how a page renders with the themes: the layout it is bound
// to with the contract's components, and the active theme's release and
// attributes table.
type PageRender struct {
	// LayoutID is the bound layout, "" for a page without a binding.
	LayoutID string
	// Layout is the layout's source.
	Layout     string
	Components []themecore.Component
	// Release is the active theme's release, "" when the page renders with
	// the fallback.
	Release string
	// Attributes is the active theme's attributes table, nil when there is no
	// active theme or it has no attributes.toml.
	Attributes *themecore.AttributeTable
	// Current resolves the current of a LINK whose current is __auto. It is
	// nil when the render has no page address, as on the 404 page; __auto is
	// then empty.
	Current compono.CurrentResolverFunc
}

// Options returns the Compono options of the render: UMONO_LAYOUT as an
// isolated global whose sub components are the contract's components, the
// injection hook of the attributes table and the resolver of the current of
// the links whose current is __auto. A page without a binding has no layout
// and no injection hook, but still the resolver when it has one.
func (r PageRender) Options() []compono.ConvertOption {
	var options []compono.ConvertOption
	if r.LayoutID != "" {
		sub := []compono.ConvertOption{compono.WithIsolatedScope()}
		for _, c := range r.Components {
			sub = append(sub, compono.WithGlobalComponent(c.Name, []byte(c.Source)))
		}
		options = append(options, compono.WithGlobalComponent(layoutGlobal, []byte(r.Layout), sub...))
		if r.Attributes != nil {
			table, layout := *r.Attributes, r.LayoutID
			options = append(options, compono.WithAttributeHook(func(builtin string, chain []compono.Frame) map[string]string {
				return table.Lookup(layout, builtin, chain)
			}))
		}
	}
	if r.Current != nil {
		options = append(options, compono.WithCurrentResolver(r.Current))
	}
	return options
}

// RootClass returns the class of <umono-root>: the class of the layout
// address in the attributes table, or "".
func (r PageRender) RootClass() string {
	if r.Attributes == nil || r.LayoutID == "" {
		return ""
	}
	return r.Attributes.Root(r.LayoutID)["class"]
}

// PageRender resolves how page renders. A page without a binding renders
// without a layout and with the fallback. A bound page renders with the
// record of its group's active theme when the group has a loaded one, and
// with its own record and the fallback otherwise, also when the theme is
// broken. Coverage makes the active theme's record safe: it has every
// layout and parameter of the older records of its group.
func (s *ThemeService) PageRender(page models.SitePage) (PageRender, error) {
	if page.ContractID == nil || page.LayoutID == nil {
		return PageRender{}, nil
	}
	record, ok := s.contracts.ByID(*page.ContractID)
	if !ok {
		return PageRender{}, fmt.Errorf("%w: %d", ErrContractNotFound, *page.ContractID)
	}

	var r PageRender
	var contract themecore.Contract
	if loaded, ok := s.active.Get(record.Group()); ok {
		contract = loaded.Contract
		if loaded.Path == theme.DevPath {
			r.Release, r.Attributes = s.devRender()
		} else {
			r.Release = loaded.Release
			if loaded.HasAttributes {
				table := loaded.Attributes
				r.Attributes = &table
			}
		}
	} else {
		parsed, err := s.parsedContract(record)
		if err != nil {
			return PageRender{}, err
		}
		contract = parsed
	}

	for _, layout := range contract.Layouts {
		if layout.ID == *page.LayoutID {
			r.LayoutID = layout.ID
			r.Layout = layout.Source
			r.Components = contract.Components
			return r, nil
		}
	}
	return PageRender{}, fmt.Errorf("%w: %s %s", ErrLayoutNotFound, record.Ref(), *page.LayoutID)
}

// devRender returns the release and the attributes table of the theme in
// development, checked on every render (privileges #5 and #9). A page
// renders without injection when attributes.toml fails, and with the
// fallback on a fatal error of the check.
func (s *ThemeService) devRender() (string, *themecore.AttributeTable) {
	r, err := theme.CheckDev(s.themesDir, s.devRoot)
	if err != nil {
		return "", nil
	}
	if r.AttributesErr != nil {
		return theme.DevRelease, nil
	}
	return theme.DevRelease, &r.Attributes
}

// parsedContract returns the record parsed with themecore, once per record.
func (s *ThemeService) parsedContract(record models.Contract) (themecore.Contract, error) {
	s.parsedMu.Lock()
	defer s.parsedMu.Unlock()
	if c, ok := s.parsed[record.ID]; ok {
		return c, nil
	}
	c, err := themecore.ParseContract([]byte(record.Source))
	if err != nil {
		return themecore.Contract{}, err
	}
	s.parsed[record.ID] = c
	return c, nil
}
