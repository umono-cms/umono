package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/umono-cms/umono/internal/models"
)

// link returns the Compono source of a LINK whose current is resolved against
// the address of the page that renders it.
func link(url string) string {
	return fmt.Sprintf("{{ LINK text = %sx%s url = %s%s%s current = %s__auto%s }}%s",
		quote, quote, quote, url, quote, quote, quote, "\n\n")
}

// marked reports whether the content has the link of the url marked as the
// address of the page that renders it.
func marked(content, url string) bool {
	return strings.Contains(content, fmt.Sprintf("<a href=%s%s%s aria-current=%spage%s>",
		quote, url, quote, quote, quote))
}

func TestCurrentAutoMarksThePageAddress(t *testing.T) {
	ctx := context.Background()
	sps, _, db := newRenderTestServices(t)

	tests := []struct {
		url  string
		want bool
	}{
		{"/about", true},
		{"/about/", true},
		{"/about?x=1#top", true},
		{"/", false},
		{"/other", false},
		{"about", false},
		{"https://example.com/about", false},
		{"//example.com/about", false},
	}

	content := ""
	for _, tc := range tests {
		content += link(tc.url)
	}
	createPage(t, db, models.SitePage{Name: "About", Slug: "about", Enabled: true, Content: content})

	r, err := sps.GetRenderedBySlug(ctx, "about")
	if err != nil {
		t.Fatalf("GetRenderedBySlug: %v", err)
	}

	for _, tc := range tests {
		if got := marked(r.Content, tc.url); got != tc.want {
			t.Errorf("marked(r.Content, %q) = %v, want %v; r.Content = %q", tc.url, got, tc.want, r.Content)
		}
	}
}

func TestCurrentAutoMarksTheHomePage(t *testing.T) {
	ctx := context.Background()
	sps, _, db := newRenderTestServices(t)

	createPage(t, db, models.SitePage{Name: "Home", Slug: "", Enabled: true, Content: link("/") + link("/?q=1") + link("/about")})

	r, err := sps.GetRenderedBySlug(ctx, "")
	if err != nil {
		t.Fatalf("GetRenderedBySlug: %v", err)
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"/", true},
		{"/?q=1", true},
		{"/about", false},
	}

	for _, tc := range tests {
		if got := marked(r.Content, tc.url); got != tc.want {
			t.Errorf("marked(r.Content, %q) = %v, want %v; r.Content = %q", tc.url, got, tc.want, r.Content)
		}
	}
}

func TestCurrentAutoOnTheNotFoundPage(t *testing.T) {
	ctx := context.Background()
	sps, _, _ := newRenderTestServices(t)

	r, err := sps.GetNotFoundPage(ctx, "Default", link("/")+link("/missing"))
	if err != nil {
		t.Fatalf("GetNotFoundPage: %v", err)
	}

	if strings.Contains(r.Content, "aria-current") {
		t.Errorf("r.Content contains %q; r.Content = %q", "aria-current", r.Content)
	}
}
