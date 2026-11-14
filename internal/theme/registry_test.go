package theme

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
)

// signedRef is the reference of the signed contract fixture.
var signedRef = models.Contract{Org: "umono", Name: "page", Major: 0, Minor: 1}

// readSignedContract returns the bytes of the published contract fixture.
func readSignedContract(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/signed/contract.toml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	return src
}

// readSignedSignature returns the bytes of the signature of the contract
// fixture.
func readSignedSignature(t *testing.T) []byte {
	t.Helper()
	sig, err := os.ReadFile("testdata/signed/contract.toml.minisig")
	if err != nil {
		t.Fatalf("read signature: %v", err)
	}
	return sig
}

// flipBase64Char replaces the character at index of a base64 line with
// another base64 character, so the line stays valid base64.
func flipBase64Char(line string, index int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(line)
	old := b[index]
	repl := byte(alphabet[0])
	if old == repl {
		repl = alphabet[1]
	}
	b[index] = repl
	return string(b)
}

func TestRegistryVerify(t *testing.T) {
	src := readSignedContract(t)
	sig := readSignedSignature(t)

	var saved *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saved = r.Clone(context.Background())
		_, _ = w.Write(sig)
	}))
	defer srv.Close()

	ctx := context.Background()

	if err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if saved.Method != "GET" {
		t.Fatalf("method = %q, want GET", saved.Method)
	}
	if got, want := saved.URL.Path, "/registry/v1/contracts/umono/page/v0.1.minisig"; got != want {
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

func TestRegistryVerifyRejectsChangedContract(t *testing.T) {
	src := append(readSignedContract(t), '\n')
	sig := readSignedSignature(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sig)
	}))
	defer srv.Close()

	ctx := context.Background()

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, themecore.ErrContractSignature) {
		t.Fatalf("err = %v, want ErrContractSignature", err)
	}
}

func TestRegistryVerifyRejectsChangedSignature(t *testing.T) {
	src := readSignedContract(t)

	lines := strings.Split(strings.TrimSuffix(string(readSignedSignature(t)), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("signature has %d lines, want 4", len(lines))
	}
	lines[1] = flipBase64Char(lines[1], 30)
	changed := []byte(strings.Join(lines, "\n") + "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(changed)
	}))
	defer srv.Close()

	ctx := context.Background()

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, themecore.ErrContractSignature) {
		t.Fatalf("err = %v, want ErrContractSignature", err)
	}
}

func TestRegistryVerifyRejectsAnotherRef(t *testing.T) {
	src := readSignedContract(t)
	sig := readSignedSignature(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sig)
	}))
	defer srv.Close()

	ref := models.Contract{Org: "umono", Name: "blog", Major: 0, Minor: 1}
	ctx := context.Background()

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, ref, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, themecore.ErrContractSignature) {
		t.Fatalf("err = %v, want ErrContractSignature", err)
	}
}

func TestRegistryVerifyReadsAtMost4KiB(t *testing.T) {
	src := readSignedContract(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 8192))
	}))
	defer srv.Close()

	ctx := context.Background()

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src)
	if err == nil {
		t.Fatalf("Verify returned nil, want error")
	}
	if !errors.Is(err, themecore.ErrContractSignature) {
		t.Fatalf("err = %v, want ErrContractSignature", err)
	}
	if errors.Is(err, ErrRegistryUnreachable) {
		t.Fatalf("err = %v, want no ErrRegistryUnreachable", err)
	}
}

func TestRegistryVerifyNotPublished(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	ctx := context.Background()

	src := readSignedContract(t)

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src)
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

	ctx := context.Background()

	src := readSignedContract(t)

	err := (Registry{BaseURL: srv.URL, Client: srv.Client()}).Verify(ctx, signedRef, src)
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

	ctx := context.Background()

	src := readSignedContract(t)

	err := (Registry{BaseURL: baseURL, Client: client}).Verify(ctx, signedRef, src)
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

func TestRegistryHost(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "official registry", input: "https://umono.dev", want: "umono.dev"},
		{name: "mirror", input: "https://registry.example.lan", want: "registry.example.lan"},
		{name: "host with port", input: "http://10.0.0.5:8080/umono", want: "10.0.0.5:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRegistry(tt.input).Host(); got != tt.want {
				t.Fatalf("Host() = %q, want %q", got, tt.want)
			}
		})
	}
}
