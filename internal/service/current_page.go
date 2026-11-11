package service

import (
	"net/url"
	"strings"

	"github.com/umono-cms/compono"
)

// pageAddress returns the address of the page with the slug: the slug under
// the root, so that the home page, whose slug is empty, has the address "/".
func pageAddress(slug string) string {
	return "/" + slug
}

// currentPage returns the resolver that decides the current of a LINK whose
// current is __auto against the address of the page that renders it: "page"
// for the link whose address is the page's address, "" for every other
// link. A link to another scheme or another host is never the page itself,
// and a link with one trailing slash is the page it addresses without it.
func currentPage(address string) compono.CurrentResolverFunc {
	return func(rawurl string) string {
		u, err := url.Parse(rawurl)
		if err != nil || u.Scheme != "" || u.Host != "" {
			return ""
		}
		path := u.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			path = path[:len(path)-1]
		}
		if path == address {
			return "page"
		}
		return ""
	}
}
