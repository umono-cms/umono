package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	umonocrypto "github.com/umono-cms/crypto"
	"github.com/umono-cms/umono/internal/credentials"
	"github.com/umono-cms/umono/internal/theme"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

	db, err := gorm.Open(sqlite.Open("umono.db"), &gorm.Config{})
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

func updateEnvFile(envFile map[string]string, username, password string) error {
	hashedUsername, err := credentials.Hash(username)
	if err != nil {
		return err
	}

	hashedPassword, err := credentials.Hash(password)
	if err != nil {
		return err
	}

	content := ""
	content += "APP_ENV=" + os.Getenv("APP_ENV") + "\n"
	content += "SESSION_DRIVER=" + os.Getenv("SESSION_DRIVER") + "\n\n"

	content += "PORT=" + os.Getenv("PORT") + "\n"
	content += "DSN=" + os.Getenv("DSN") + "\n\n"

	if umonoSecret := strings.TrimSpace(envFile["UMONO_SECRET"]); umonoSecret != "" {
		content += "UMONO_SECRET=" + umonoSecret + "\n\n"
	}

	content += "USERNAME=\n"
	content += "PASSWORD=\n\n"

	content += "HASHED_USERNAME=" + credentials.Encode(hashedUsername) + "\n"
	content += "HASHED_PASSWORD=" + credentials.Encode(hashedPassword) + "\n\n"

	file, err := os.OpenFile(".env", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	if err := godotenv.Overload(); err != nil {
		return err
	}
	os.Unsetenv("UMONO_SECRET")

	return nil
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
