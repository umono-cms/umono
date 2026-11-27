package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	umonocrypto "github.com/umono-cms/crypto"
	"github.com/umono-cms/umono/internal/credentials"
	"github.com/umono-cms/umono/internal/theme"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	errUmonoSecretNotSet = errors.New("UMONO_SECRET is not set. Run: umono secret init")

	errCredentialsNotSet = errors.New(
		"USERNAME and PASSWORD are not set. Set both in .env, then start Umono again")
	errCredentialsIncomplete = errors.New(
		"USERNAME and PASSWORD must be set together in .env, then start Umono again")
	errCredentialsEmpty = errors.New(
		"HASHED_USERNAME and HASHED_PASSWORD are hashes of an empty credential. " +
			"Set USERNAME and PASSWORD in .env, then start Umono again")
)

func main() {
	if err := godotenv.Load(); err != nil {
		panic("Error loading .env file!")
	}

	umonoSecret, err := newUmonoSecretFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := ensureAdminCredentials(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	devRoot, err := devRootFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	registryURL, err := registryURLFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	db, err := openDB("umono.db", os.Stdout)
	if err != nil {
		log.Fatal("db err", err)
	}

	app, err := newApp(db, umonoSecret, devRoot, registryURL)
	if err != nil {
		log.Fatal(err)
	}

	port := ":8999"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	log.Fatal(app.Listen(port))
}

// openDB opens the SQLite database at path. GORM writes slow queries and
// database errors to w, but not the expected "record not found" results of
// First, Take and Last, which a new project returns at startup and while the
// job queue is empty.
func openDB(path string, w io.Writer) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.New(log.New(w, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		}),
	})
}

// devRootFromEnv returns the theme project of the dev mode from
// UMONO_THEME_DEV, "" when the variable is not set. A set value must be the
// absolute path of a directory, or Umono does not start.
func devRootFromEnv() (string, error) {
	v, ok := os.LookupEnv("UMONO_THEME_DEV")
	if !ok {
		return "", nil
	}
	return theme.ParseDevRoot(v)
}

// registryURLFromEnv returns the registry from UMONO_REGISTRY, the registry at
// theme.RegistryURL (umono.dev) when the variable is not set. A set value must
// be a valid root URL, or Umono does not start.
func registryURLFromEnv() (string, error) {
	v, ok := os.LookupEnv("UMONO_REGISTRY")
	if !ok {
		return theme.RegistryURL, nil
	}
	return theme.ParseRegistryURL(v)
}

func newUmonoSecretFromEnv() (*umonocrypto.Secret, error) {
	secretValue := strings.TrimSpace(os.Getenv("UMONO_SECRET"))
	if secretValue == "" {
		return nil, errUmonoSecretNotSet
	}
	defer os.Unsetenv("UMONO_SECRET")

	key, err := umonocrypto.ParseHexString(secretValue)
	if err != nil {
		return nil, err
	}

	return umonocrypto.New(key, []byte("umono-secrets"))
}

// updateEnvFile clears the plaintext credentials in the .env file and writes
// their hashes. Every other line of the .env file is left as it is.
func updateEnvFile(envFile map[string]string, username, password string) error {
	hashedUsername, err := credentials.Hash(username)
	if err != nil {
		return err
	}

	hashedPassword, err := credentials.Hash(password)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	updated := setEnvLines(string(content), [][2]string{
		{credentials.UsernameEnvKey, ""},
		{credentials.PasswordEnvKey, ""},
		{credentials.HashedUsernameEnvKey, credentials.Encode(hashedUsername)},
		{credentials.HashedPasswordEnvKey, credentials.Encode(hashedPassword)},
	})

	file, err := os.OpenFile(".env", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(updated); err != nil {
		return err
	}

	if err := godotenv.Overload(); err != nil {
		return err
	}
	os.Unsetenv("UMONO_SECRET")

	return nil
}

// setEnvLines writes the given key values into the content of an env file.
// The line of a key that is written is replaced with the key and its value,
// every other line is kept as it is. Keys that the content does not have yet
// are appended at the end, each one on its own line.
func setEnvLines(content string, values [][2]string) string {
	written := make(map[string]string, len(values))
	for _, value := range values {
		written[value[0]] = value[1]
	}

	seen := make(map[string]bool, len(values))
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		key := envLineKey(line)
		value, ok := written[key]
		if !ok {
			continue
		}

		lines[i] = key + "=" + value
		seen[key] = true
	}

	updated := strings.Join(lines, "\n")

	for _, value := range values {
		if seen[value[0]] {
			continue
		}

		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}

		updated += value[0] + "=" + value[1] + "\n"
		seen[value[0]] = true
	}

	return updated
}

// envLineKey returns the key of an env file line, "" for a line without a key,
// a comment line and a line without a value.
func envLineKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	key, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return ""
	}

	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "export ")

	return strings.TrimSpace(key)
}

func ensureAdminCredentials() error {
	envFile, err := godotenv.Read(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	username := plaintextCredential(envFile, credentials.UsernameEnvKey)
	password := plaintextCredential(envFile, credentials.PasswordEnvKey)

	switch {
	case !credentials.IsEmpty(username) && !credentials.IsEmpty(password):
		if err := updateEnvFile(envFile, username, password); err != nil {
			return fmt.Errorf("error updating .env file: %w", err)
		}
	case !credentials.IsEmpty(username) || !credentials.IsEmpty(password):
		return errCredentialsIncomplete
	}

	hashedUsername, hashedPassword, err := credentials.LoadHashes()
	if err != nil {
		return errCredentialsNotSet
	}

	if credentials.Matches(hashedUsername, "") || credentials.Matches(hashedPassword, "") {
		return errCredentialsEmpty
	}

	return nil
}

func plaintextCredential(envFile map[string]string, key string) string {
	if value, ok := envFile[key]; ok {
		return value
	}

	return os.Getenv(key)
}
