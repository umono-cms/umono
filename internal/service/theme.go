package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/umono-cms/themecore"
	"github.com/umono-cms/umono/internal/models"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/theme"
)

var (
	// ErrContractChanged is returned when a package's contract.toml differs
	// from the stored record of the same contract version.
	ErrContractChanged = errors.New("contract.toml differs from the stored contract of the same version")
	// ErrNoActiveTheme is returned when a contract group has no active theme.
	ErrNoActiveTheme = errors.New("the contract group has no active theme")
	// ErrActiveContractChanged is returned when an active theme's built.toml
	// names another contract than active_themes.
	ErrActiveContractChanged = errors.New("the installed theme's contract is not the one active_themes names")
	// ErrNotCovered is returned when a theme's contract does not cover the
	// base of its group.
	ErrNotCovered = errors.New("the theme's contract does not cover the base of its group")
	// ErrNotInstalled is returned for a pending install without an installed
	// theme at its path. It can only be rejected.
	ErrNotInstalled = errors.New("no theme is installed at the path of the pending install")
	// ErrGroupChanged is returned when the pending install of an active theme
	// has a contract of another group. The theme is deactivated first.
	ErrGroupChanged = errors.New("the pending install's contract is in another group; deactivate the theme first")
	// ErrContractHasThemes is returned when a theme under themes/ has the
	// contract of a record that is being deleted.
	ErrContractHasThemes = errors.New("a theme under themes/ has the contract")
	// ErrContractHasPages is returned when pages are bound to a record that
	// is being deleted.
	ErrContractHasPages = errors.New("pages are bound to the contract record")
	// ErrDevTheme is returned for an action on the group of the theme in
	// development: in dev mode it is always the group's active theme.
	ErrDevTheme = errors.New("the group's active theme is the theme in development")
)

// ContractRegistry verifies a package's contract.toml against the published
// contract. theme.Registry is the registry of umono.dev, or the mirror that
// UMONO_REGISTRY names.
type ContractRegistry interface {
	Verify(ctx context.Context, ref models.Contract, src []byte) error
	// Host is the host the registry is asked on.
	Host() string
}

// ThemeCandidate is an installed theme that passed every check of
// activation.
type ThemeCandidate struct {
	theme.Installed
	// Contract is the record of the theme's contract: the stored one, or a new
	// one with ID 0 that activation adds.
	Contract models.Contract
	// Asked tells whether the registry was asked.
	Asked bool
}

// ThemeService activates the themes installed under themesDir and applies the pending installs under pendingDir.
type ThemeService struct {
	themesDir  string
	pendingDir string
	contracts  *ContractService
	themes     *repository.ThemeRepository
	registry   ContractRegistry
	active     *theme.ActiveSet
	// activateMu serializes activations, so two of them do not add the same
	// contract record or overwrite each other's active_themes.
	activateMu         sync.Mutex
	renderInputChanged func(ctx context.Context)
	// parsedMu guards parsed, the contract records parsed with themecore by
	// ID. A record never changes, so it is parsed once.
	parsedMu sync.Mutex
	parsed   map[uint]themecore.Contract
	// devRoot is the theme project of the dev mode, UMONO_THEME_DEV, "" without
	// the dev mode.
	devRoot string
	// devGroup is the group of the theme in development and devContract the
	// reference of its contract when devHasGroup. devErr is why LoadDev
	// failed. LoadDev sets them once at startup, before Umono serves.
	devGroup    models.ContractGroup
	devContract string
	devHasGroup bool
	devErr      error
}

func NewThemeService(themesDir, pendingDir string, contracts *ContractService, themes *repository.ThemeRepository, registry ContractRegistry) *ThemeService {
	return &ThemeService{
		themesDir:  themesDir,
		pendingDir: pendingDir,
		contracts:  contracts,
		themes:     themes,
		registry:   registry,
		active:     theme.NewActiveSet(),
		parsed:     map[uint]themecore.Contract{},
	}
}

// RegistryHost returns the host of the registry activation asks.
func (s *ThemeService) RegistryHost() string { return s.registry.Host() }

// Check runs the checks of activation on the installed theme at path, in
// their order: the package checks, the contract check and the coverage check.
// The registry is asked only when the database has no record of the
// contract; a stored record never changes, so its checksum is enough.
func (s *ThemeService) Check(ctx context.Context, path string) (ThemeCandidate, error) {
	installed, err := theme.CheckInstalled(s.themesDir, path)
	if err != nil {
		return ThemeCandidate{}, err
	}
	return s.checkContract(ctx, installed)
}

// checkContract runs the contract check and the coverage check on a theme
// that passed the package checks.
func (s *ThemeService) checkContract(ctx context.Context, installed theme.Installed) (ThemeCandidate, error) {
	ref, err := models.ParseContractRef(installed.Built.Contract)
	if err != nil {
		return ThemeCandidate{}, err
	}
	sum := sha256.Sum256(installed.ContractSource)
	checksum := hex.EncodeToString(sum[:])

	record, ok := s.contracts.Record(ref)
	asked := false
	if ok {
		if record.Checksum != checksum {
			return ThemeCandidate{}, fmt.Errorf("%w: %s", ErrContractChanged, ref.Ref())
		}
	} else {
		if err := s.registry.Verify(ctx, ref, installed.ContractSource); err != nil {
			return ThemeCandidate{}, err
		}
		record = models.Contract{
			Org:      ref.Org,
			Name:     ref.Name,
			Major:    ref.Major,
			Minor:    ref.Minor,
			Source:   string(installed.ContractSource),
			Checksum: checksum,
		}
		asked = true
	}

	if !s.contracts.Selectable(ref) {
		return ThemeCandidate{}, fmt.Errorf("%w: %d pages are bound to a newer minor", ErrNotCovered, len(s.contracts.UncoveredPages(ref)))
	}
	return ThemeCandidate{Installed: installed, Contract: record, Asked: asked}, nil
}

// Activate runs the checks of activation on the installed theme at path and
// activates it: the contract record, when new, and active_themes are written
// in one transaction, and the theme replaces the active theme of its group in
// memory with its release and attributes table. The release is never stored.
func (s *ThemeService) Activate(ctx context.Context, path string) (ThemeCandidate, error) {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	c, err := s.Check(ctx, path)
	if err != nil {
		return ThemeCandidate{}, err
	}
	if s.isDevGroup(c.Contract.Group()) {
		return ThemeCandidate{}, fmt.Errorf("%w: %s", ErrDevTheme, c.Contract.Group())
	}
	if err := s.themes.Activate(&c.Contract, models.ThemeOption{Path: c.Path, ContractID: c.Contract.Ref()}); err != nil {
		return ThemeCandidate{}, err
	}
	s.active.Set(c.Contract.Group(), theme.Loaded{
		Path:          c.Path,
		Release:       c.Release,
		ContractID:    c.Contract.ID,
		Contract:      c.Result.Contract,
		Attributes:    c.Result.Attributes,
		HasAttributes: c.HasAttributes,
		Assets:        c.Assets,
	})
	s.notifyRenderInputChange()
	return c, nil
}

// Loaded returns the active theme of the group g loaded into memory.
func (s *ThemeService) Loaded(g models.ContractGroup) (theme.Loaded, bool) {
	return s.active.Get(g)
}

// UncoveredPages returns the pages of the group of ref that are bound to a
// newer minor than ref. A theme of ref cannot be selected while there are
// any.
func (s *ThemeService) UncoveredPages(ref models.Contract) []models.SitePage {
	return s.contracts.UncoveredPages(ref)
}

// Deactivate returns the group g to fallback render: its element is removed
// from active_themes and its theme from memory. The theme directory, the
// contract record and the bindings of the pages stay. It has no
// precondition.
func (s *ThemeService) Deactivate(g models.ContractGroup) error {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	if s.isDevGroup(g) {
		return fmt.Errorf("%w: %s", ErrDevTheme, g)
	}
	if err := s.themes.Deactivate(g); err != nil {
		return err
	}
	s.active.Delete(g)
	s.notifyRenderInputChange()
	return nil
}

// OnRenderInputChange sets fn to be called after an activation or a
// deactivation, which changes how the pages of a group render.
func (s *ThemeService) OnRenderInputChange(fn func(ctx context.Context)) {
	s.renderInputChanged = fn
}

func (s *ThemeService) notifyRenderInputChange() {
	if s.renderInputChanged != nil {
		s.renderInputChanged(context.Background())
	}
}

// ApplyPending uses the new content of the pending install at path. The
// checks of activation run on the pending directory; for a theme that is not
// active the contract and coverage checks are skipped, because activation
// runs them. The new contract of an active theme must stay in its group. A
// changed contract is written to active_themes first, then the pending
// directory replaces the installed one and the new release and attributes
// table replace the old ones in memory. When a step fails, the installed
// theme stays in use.
func (s *ThemeService) ApplyPending(ctx context.Context, path string) error {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	installed, err := theme.CheckInstalled(s.pendingDir, path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.themesDir, filepath.FromSlash(path))); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotInstalled, path)
		}
		return err
	}

	g, old, active := s.active.ByPath(path)
	if !active {
		return theme.Replace(s.themesDir, s.pendingDir, path)
	}

	ref, err := models.ParseContractRef(installed.Built.Contract)
	if err != nil {
		return err
	}
	if ref.Group() != g {
		return fmt.Errorf("%w: %s", ErrGroupChanged, ref.Ref())
	}
	c, err := s.checkContract(ctx, installed)
	if err != nil {
		return err
	}

	contractChanged := c.Contract.ID != old.ContractID
	if contractChanged {
		if err := s.themes.Activate(&c.Contract, models.ThemeOption{Path: path, ContractID: c.Contract.Ref()}); err != nil {
			return err
		}
	}
	if err := theme.Replace(s.themesDir, s.pendingDir, path); err != nil {
		if contractChanged {
			previous := models.Contract{ID: old.ContractID}
			_ = s.themes.Activate(&previous, models.ThemeOption{Path: path, ContractID: old.Contract.ID + "@" + old.Contract.Version})
		}
		return err
	}

	s.active.Set(g, theme.Loaded{
		Path:          path,
		Release:       c.Release,
		ContractID:    c.Contract.ID,
		Contract:      c.Result.Contract,
		Attributes:    c.Result.Attributes,
		HasAttributes: c.HasAttributes,
		Assets:        c.Assets,
	})
	if contractChanged {
		s.notifyRenderInputChange()
	}
	return nil
}

// LoadActive loads the themes of active_themes into memory at startup. Each
// one reruns the checks of activation that need no network (loadActive). A
// theme that fails them is marked broken and logged, and its group renders
// with the fallback; nothing is written. Only an unreadable active_themes is
// returned as an error.
func (s *ThemeService) LoadActive(ctx context.Context) error {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	themes, err := s.themes.GetActiveThemes()
	if err != nil {
		return err
	}
	for _, t := range themes {
		ref, err := models.ParseContractRef(t.ContractID)
		if err != nil {
			log.Printf("error: active theme %s: %v", t.Path, err)
			continue
		}
		if err := s.loadActive(ref, t.Path); err != nil {
			log.Printf("error: active theme %s: %v", t.Path, err)
		}
	}
	return nil
}

// Retry reruns the checks of loading for the broken active theme of the
// group g. On success the theme is used again.
func (s *ThemeService) Retry(ctx context.Context, g models.ContractGroup) error {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	if s.isDevGroup(g) {
		return fmt.Errorf("%w: %s", ErrDevTheme, g)
	}
	themes, err := s.themes.GetActiveThemes()
	if err != nil {
		return err
	}
	for _, t := range themes {
		ref, err := models.ParseContractRef(t.ContractID)
		if err != nil || ref.Group() != g {
			continue
		}
		if err := s.loadActive(ref, t.Path); err != nil {
			return err
		}
		s.notifyRenderInputChange()
		return nil
	}
	return ErrNoActiveTheme
}

// Broken returns the broken active theme of the group g.
func (s *ThemeService) Broken(g models.ContractGroup) (theme.Broken, bool) {
	return s.active.Broken(g)
}

// loadActive runs the checks of activation that need no network on the
// active theme of ref at path: the package checks, the contract of
// built.toml against active_themes and contract.toml against the stored
// record's checksum. The contract and coverage checks are not repeated: the
// record never changes and no page can be bound above an active theme. It
// loads the theme or marks it broken.
func (s *ThemeService) loadActive(ref models.Contract, path string) error {
	l, err := s.checkActive(ref, path)
	if err != nil {
		s.active.SetBroken(ref.Group(), theme.Broken{Path: path, Err: err})
		return err
	}
	s.active.Set(ref.Group(), l)
	return nil
}

func (s *ThemeService) checkActive(ref models.Contract, path string) (theme.Loaded, error) {
	installed, err := theme.CheckInstalled(s.themesDir, path)
	if err != nil {
		return theme.Loaded{}, err
	}
	if installed.Built.Contract != ref.Ref() {
		return theme.Loaded{}, fmt.Errorf("%w: %s, not %s", ErrActiveContractChanged, installed.Built.Contract, ref.Ref())
	}
	record, ok := s.contracts.Record(ref)
	if !ok {
		return theme.Loaded{}, fmt.Errorf("%w: %s", ErrContractNotFound, ref.Ref())
	}
	sum := sha256.Sum256(installed.ContractSource)
	if hex.EncodeToString(sum[:]) != record.Checksum {
		return theme.Loaded{}, fmt.Errorf("%w: %s", ErrContractChanged, ref.Ref())
	}
	return theme.Loaded{
		Path:          path,
		Release:       installed.Release,
		ContractID:    record.ID,
		Contract:      installed.Result.Contract,
		Attributes:    installed.Result.Attributes,
		HasAttributes: installed.HasAttributes,
		Assets:        installed.Assets,
	}, nil
}

// DeleteContract deletes the stored record of the contract ref, the site
// owner's "Delete record" action; a record is never deleted otherwise. Right
// before, it checks again that no theme under themes/, active or not, has the
// contract and that no page, disabled ones included, is bound to the record.
// A theme whose built.toml cannot be read refuses the deletion, because its
// contract is unknown. SQLite does not enforce the foreign key, so the pages
// are counted here. In dev mode the theme in development has the contract of
// its startup check.
func (s *ThemeService) DeleteContract(ref models.Contract) error {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()

	record, ok := s.contracts.Record(ref)
	if !ok {
		return fmt.Errorf("%w: %s", ErrContractNotFound, ref.Ref())
	}
	installed, err := theme.ListInstalled(s.themesDir)
	if err != nil {
		return err
	}
	for _, t := range installed {
		if s.devRoot != "" && t.Path == theme.DevPath {
			if !s.devHasGroup {
				return fmt.Errorf("theme %s: %w", t.Path, s.devErr)
			}
			if s.devContract == ref.Ref() {
				return fmt.Errorf("%w: %s", ErrContractHasThemes, t.Path)
			}
			continue
		}
		if t.Err != nil {
			return fmt.Errorf("theme %s: %w", t.Path, t.Err)
		}
		if t.Built.Contract == ref.Ref() {
			return fmt.Errorf("%w: %s", ErrContractHasThemes, t.Path)
		}
	}
	if n := s.contracts.CountPages(record.ID); n > 0 {
		return fmt.Errorf("%w: %d pages", ErrContractHasPages, n)
	}
	return s.contracts.Delete(record.ID)
}

// EnableDev turns the dev mode on with root, the theme project. Only then
// does dev/dev get its privileges.
func (s *ThemeService) EnableDev(root string) {
	s.devRoot = root
}

// LoadDev loads the theme in development at startup, after LoadActive. It
// does nothing without the dev mode. No check needs the network: a stored
// record of the contract must have contract.toml's checksum, and without one
// the record is added from contract.toml without asking the registry. The
// coverage check runs as in production. On success dev/dev is the active
// theme of its group in memory; active_themes is neither read nor written. A
// failure after the group is known marks the group broken, so it renders
// with the fallback.
func (s *ThemeService) LoadDev(ctx context.Context) (err error) {
	if s.devRoot == "" {
		return nil
	}
	s.activateMu.Lock()
	defer s.activateMu.Unlock()
	defer func() { s.devErr = err }()

	var r themecore.DevResult
	r, err = theme.CheckDev(s.themesDir, s.devRoot)
	if err != nil {
		return err
	}
	var ref models.Contract
	ref, err = models.ParseContractRef(r.Built.Contract)
	if err != nil {
		return err
	}
	s.devGroup, s.devContract, s.devHasGroup = ref.Group(), ref.Ref(), true

	sum := sha256.Sum256(r.ContractSource)
	checksum := hex.EncodeToString(sum[:])
	record, ok := s.contracts.Record(ref)
	switch {
	case ok && record.Checksum != checksum:
		err = fmt.Errorf("%w: %s", ErrContractChanged, ref.Ref())
	case !s.contracts.Selectable(ref):
		err = fmt.Errorf("%w: %d pages are bound to a newer minor", ErrNotCovered, len(s.contracts.UncoveredPages(ref)))
	case !ok:
		record = models.Contract{
			Org:      ref.Org,
			Name:     ref.Name,
			Major:    ref.Major,
			Minor:    ref.Minor,
			Source:   string(r.ContractSource),
			Checksum: checksum,
		}
		err = s.contracts.Add(&record)
	}
	if err != nil {
		s.active.SetBroken(ref.Group(), theme.Broken{Path: theme.DevPath, Err: err})
		return err
	}
	s.active.Set(ref.Group(), theme.Loaded{
		Path:       theme.DevPath,
		Release:    theme.DevRelease,
		ContractID: record.ID,
		Contract:   r.Contract,
	})
	return nil
}

// isDevGroup reports whether g is the group of the theme in development.
func (s *ThemeService) isDevGroup(g models.ContractGroup) bool {
	return s.devHasGroup && s.devGroup == g
}

// Asset returns the asset name, a path relative to assets/, of the loaded
// active theme whose release is release. A broken or deactivated theme has
// none.
func (s *ThemeService) Asset(release, name string) ([]byte, bool) {
	t, ok := s.active.ByRelease(release)
	if !ok {
		return nil, false
	}
	b, ok := t.Assets[name]
	return b, ok
}

// DevAsset returns assets/name of the theme in development, checked on
// every request (privileges #4 and #6). theme.css is the Transform output of
// assets/theme.css. ok is false without a loaded theme in development and
// for a file that is missing or breaks the rules of assets/. err is a fatal
// error of the check or, for theme.css, its CSS error.
func (s *ThemeService) DevAsset(name string) (b []byte, ok bool, err error) {
	if _, loaded := s.active.ByRelease(theme.DevRelease); !loaded {
		return nil, false, nil
	}
	r, err := theme.CheckDev(s.themesDir, s.devRoot)
	if err != nil {
		return nil, false, err
	}
	if name == "theme.css" {
		if r.CSSErr != nil {
			return nil, false, r.CSSErr
		}
		return r.CSS, true, nil
	}
	if _, bad := r.AssetErrs["assets/"+name]; bad {
		return nil, false, nil
	}
	b, err = theme.ReadDevAsset(s.themesDir, name)
	if err != nil {
		return nil, false, nil
	}
	return b, true, nil
}

// RejectPending deletes the pending install at path, the "Reject" action.
// The installed theme and the database are not touched.
func (s *ThemeService) RejectPending(path string) error {
	return theme.RejectPending(s.pendingDir, path)
}
