package theme

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/umono-cms/umono/internal/models"
)

func TestRegistryVerify(t *testing.T) {
	src, err := os.ReadFile("testdata/package/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}

	var saved *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saved = r.Clone(context.Background())
		_, _ = w.Write(src)
	}))
	defer srv.Close()

	ref := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}
	ctx := context.Background()

	if err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, ref, src); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if saved.Method != "GET" {
		t.Fatalf("method = %q, want GET", saved.Method)
	}
	if got, want := saved.URL.Path, "/registry/v1/contracts/example/blog/v1.0"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if saved.URL.RawQuery != "" {
		t.Fatalf("raw query = %q, want empty", saved.URL.RawQuery)
	}
	if got := saved.Header.Get("User-Agent"); got != "Umono" {
		t.Fatalf("User-Agent = %q, want Umono", got)
	}
	if got := saved.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want empty", got)
	}
	if got := saved.Header.Get("Referer"); got != "" {
		t.Fatalf("Referer = %q, want empty", got)
	}
}

func TestRegistryVerifyRejectsAnotherFile(t *testing.T) {
	src, err := os.ReadFile("testdata/package/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(src)
		_, _ = w.Write([]byte("\n"))
	}))
	defer srv.Close()

	ref := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}
	ctx := context.Background()

	err = (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, ref, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, ErrContractMismatch) {
		t.Fatalf("err = %v, want ErrContractMismatch", err)
	}
}

func TestRegistryVerifyNotPublished(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	ref := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}
	ctx := context.Background()

	src, err := os.ReadFile("testdata/package/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}

	err = (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, ref, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, ErrContractNotPublished) {
		t.Fatalf("err = %v, want ErrContractNotPublished", err)
	}
}

func TestRegistryVerifyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ref := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}
	ctx := context.Background()

	src, err := os.ReadFile("testdata/package/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}

	err = (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, ref, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, ErrRegistryUnreachable) {
		t.Fatalf("err = %v, want ErrRegistryUnreachable", err)
	}
}

func TestRegistryVerifyUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := srv.URL
	client := srv.Client()
	srv.Close()

	ref := models.Contract{Org: "example", Name: "blog", Major: 1, Minor: 0}
	ctx := context.Background()

	src, err := os.ReadFile("testdata/package/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}

	err = (Registry{BaseURL: baseURL, Client: client}).Verify(ctx, ref, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, ErrRegistryUnreachable) {
		t.Fatalf("err = %v, want ErrRegistryUnreachable", err)
	}
}

func TestParseRegistryURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "https", input: "https://registry.example.lan", want: "https://registry.example.lan"},
		{name: "http", input: "http://registry.example.lan", want: "http://registry.example.lan"},
		{name: "trailing slash", input: "https://registry.example.lan/", want: "https://registry.example.lan"},
		{name: "path and trailing slash", input: "http://10.0.0.5:8080/umono/", want: "http://10.0.0.5:8080/umono"},
		{name: "empty", input: "", wantErr: true},
		{name: "missing scheme", input: "registry.example.lan", wantErr: true},
		{name: "unsupported scheme", input: "ftp://registry.example.lan", wantErr: true},
		{name: "missing host", input: "https://", wantErr: true},
		{name: "query", input: "https://registry.example.lan?x=1", wantErr: true},
		{name: "empty query", input: "https://registry.example.lan/?", wantErr: true},
		{name: "fragment", input: "https://registry.example.lan#top", wantErr: true},
		{name: "relative path", input: "/registry", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRegistryURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRegistryURL(%q) returned nil error", tt.input)
				}
				if !errors.Is(err, ErrRegistryURL) {
					t.Fatalf("err = %v, want ErrRegistryURL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRegistryURL(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseRegistryURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewRegistryUsesBaseURL(t *testing.T) {
	if got, want := NewRegistry("http://example.test").BaseURL, "http://example.test"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
}
