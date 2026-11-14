package theme

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
)

// RegistryURL is the base URL of umono.dev, the official registry.
const RegistryURL = "https://umono.dev"

// maxContractSize is themecore's size limit of contract.toml.
const maxContractSize = 1 << 20

// maxSignatureSize is the most Umono reads of a signature response.
const maxSignatureSize = 4 << 10

var (
	// ErrContractNotPublished is returned when the registry has no such contract.
	ErrContractNotPublished = errors.New("contract is not published on the registry")
	// ErrRegistryUnreachable is returned when the registry cannot answer.
	ErrRegistryUnreachable = errors.New("registry is unreachable")
	// ErrRegistryURL is returned when UMONO_REGISTRY is not a valid registry URL.
	ErrRegistryURL = errors.New("UMONO_REGISTRY must be an http or https root URL without a query or fragment")
)

// Registry verifies contracts against the registry. The request carries the
// least information: a GET of the signature's URL with the User-Agent Umono
// and no query, cookie, site address or Umono version.
type Registry struct {
	BaseURL string
	Client  *http.Client
}

// ParseRegistryURL checks the value of UMONO_REGISTRY, the root URL of a mirror
// that replaces umono.dev.
func ParseRegistryURL(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrRegistryURL, v)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(v, "#") || u.User != nil {
		return "", fmt.Errorf("%w: %q", ErrRegistryURL, v)
	}
	return strings.TrimRight(v, "/"), nil
}

// NewRegistry returns the registry at baseURL, or the registry at RegistryURL
// for umono.dev.
func NewRegistry(baseURL string) Registry {
	return Registry{BaseURL: baseURL, Client: &http.Client{Timeout: 15 * time.Second}}
}

// Host returns the host of the registry, with its port if it has one, as the
// activation confirm names it.
func (r Registry) Host() string {
	u, err := url.Parse(r.BaseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// Verify fetches the signature of ref and verifies src with it locally.
func (r Registry) Verify(ctx context.Context, ref models.Contract, src []byte) error {
	url := fmt.Sprintf("%s/registry/v1/contracts/%s/%s/v%d.%d.minisig", r.BaseURL, ref.Org, ref.Name, ref.Major, ref.Minor)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Umono")
	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRegistryUnreachable, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrContractNotPublished, ref.Ref())
	default:
		return fmt.Errorf("%w: status %d", ErrRegistryUnreachable, resp.StatusCode)
	}
	sig, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureSize))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRegistryUnreachable, err)
	}
	if err := themecore.VerifyContractSignature(src, sig, ref.Ref()); err != nil {
		return fmt.Errorf("%s: %w", ref.Ref(), err)
	}
	return nil
}
