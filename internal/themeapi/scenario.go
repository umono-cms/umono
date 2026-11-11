package themeapi

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/service"
)

// parseTxtar reads a txtar archive: text before the first "-- NAME --" line
// is a comment, and each file's content runs to the next such line or the
// end.
func parseTxtar(b []byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	var name string
	var content bytes.Buffer
	open := false
	flush := func() error {
		if !open {
			return nil
		}
		if _, ok := files[name]; ok {
			return fmt.Errorf("txtar: %s: repeated", name)
		}
		files[name] = bytes.Clone(content.Bytes())
		content.Reset()
		return nil
	}
	for _, line := range strings.SplitAfter(string(b), "\n") {
		trimmed := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(trimmed, "-- ") && strings.HasSuffix(trimmed, " --") && len(trimmed) > 6 {
			if err := flush(); err != nil {
				return nil, err
			}
			name = strings.TrimSpace(trimmed[3 : len(trimmed)-3])
			open = true
			continue
		}
		if open {
			content.WriteString(line)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return files, nil
}

// scenarioLayout returns the layout a scenario renders: the longest layout id
// that the scenario's name is, or starts with followed by "-".
func scenarioLayout(name string, layouts []string) (string, error) {
	best := ""
	for _, id := range layouts {
		if (name == id || strings.HasPrefix(name, id+"-")) && len(id) > len(best) {
			best = id
		}
	}
	if best == "" {
		return "", fmt.Errorf("scenario %s: its name starts with no layout id", name)
	}
	return best, nil
}

// renderScenario renders the scenario archive named name with the production
// render, service.ContentCompiler.RenderRoot: the page.comp of the archive is
// bound to the layout of contract.toml that scenarioLayout picks, with the
// table that themecore compiles from attributes.toml when there is one and
// the site's globals of globals/NAME.comp. The file page.slug, when the
// archive has one, holds the slug of the page the scenario renders, so that
// its links have the current of that page; without it the scenario has no
// page address, as the 404 page. It returns the canonical form of
// <umono-root>. A diagnostic is an error, because the corpus holds only
// error-free inputs.
func renderScenario(name string, archive []byte) (string, error) {
	files, err := parseTxtar(archive)
	if err != nil {
		return "", err
	}
	contract, err := themecore.ParseContract(files["contract.toml"])
	if err != nil {
		return "", err
	}
	page, ok := files["page.comp"]
	if !ok {
		return "", fmt.Errorf("page.comp: missing")
	}

	ids := make([]string, 0, len(contract.Layouts))
	for _, l := range contract.Layouts {
		ids = append(ids, l.ID)
	}
	layoutID, err := scenarioLayout(name, ids)
	if err != nil {
		return "", err
	}
	r := service.PageRender{LayoutID: layoutID, Components: contract.Components}
	for _, l := range contract.Layouts {
		if l.ID == layoutID {
			r.Layout = l.Source
		}
	}
	if b, ok := files["attributes.toml"]; ok {
		table, err := themecore.CompileAttributes(b, contract)
		if err != nil {
			return "", err
		}
		r.Attributes = &table
	}
	if b, ok := files["page.slug"]; ok {
		r.Current = service.CurrentResolver(strings.TrimSpace(string(b)))
	}

	var globals []models.Component
	for file, b := range files {
		if dir, base := path.Split(file); dir == "globals/" && strings.HasSuffix(base, ".comp") {
			globals = append(globals, models.Component{Name: strings.TrimSuffix(base, ".comp"), Content: string(b)})
		}
	}
	cc, err := service.NewContentCompiler(globals)
	if err != nil {
		return "", err
	}

	root, _, diagnostics, err := cc.RenderRoot(context.Background(), string(page), r)
	if err != nil {
		return "", err
	}
	if len(diagnostics) > 0 {
		return "", fmt.Errorf("%s: %s", diagnostics[0].Code, diagnostics[0].Message)
	}
	return Canonical([]byte(root))
}
