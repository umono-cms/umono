package main

import (
	"encoding/base64"
	"errors"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/umono-cms/umono/internal/credentials"
	"github.com/umono-cms/umono/internal/theme"
	"golang.org/x/crypto/bcrypt"
)

func TestUpdateEnvFilePreservesUmonoSecret(t *testing.T) {
	chdirTemp(t)

	const umonoSecret = "persisted-secret"
	if err := os.WriteFile(".env", []byte("UMONO_SECRET="+umonoSecret+"\nUSERNAME=admin\nPASSWORD=admin\n"), 0o666); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("APP_ENV", "dev")
	t.Setenv("SESSION_DRIVER", "db")
	t.Setenv("PORT", "8999")
	t.Setenv("DSN", "umono.db")
	t.Setenv("USERNAME", "new-admin")
	t.Setenv("PASSWORD", "new-password")

	envFileBefore, err := godotenv.Read(".env")
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	if err := updateEnvFile(envFileBefore, "new-admin", "new-password"); err != nil {
		t.Fatalf("update env file: %v", err)
	}

	envFile, err := godotenv.Read(".env")
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	if envFile["UMONO_SECRET"] != umonoSecret {
		t.Fatalf("expected UMONO_SECRET to be preserved, got %q", envFile["UMONO_SECRET"])
	}
	if os.Getenv("UMONO_SECRET") != "" {
		t.Fatal("expected UMONO_SECRET to stay out of process env")
	}
	if envFile["USERNAME"] != "" {
		t.Fatalf("expected USERNAME to be cleared, got %q", envFile["USERNAME"])
	}
	if envFile["PASSWORD"] != "" {
		t.Fatalf("expected PASSWORD to be cleared, got %q", envFile["PASSWORD"])
	}

	hashedUsername, err := base64.StdEncoding.DecodeString(envFile["HASHED_USERNAME"])
	if err != nil {
		t.Fatalf("decode hashed username: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword(hashedUsername, []byte("new-admin")); err != nil {
		t.Fatalf("expected username hash to match: %v", err)
	}

	hashedPassword, err := base64.StdEncoding.DecodeString(envFile["HASHED_PASSWORD"])
	if err != nil {
		t.Fatalf("decode hashed password: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte("new-password")); err != nil {
		t.Fatalf("expected password hash to match: %v", err)
	}
}

func TestEnsureAdminCredentialsHashesPlaintextPair(t *testing.T) {
	chdirTemp(t)
	writeEnvFile(t, "USERNAME=new-admin\nPASSWORD=new-password\n")
	setCredentialEnv(t, "host-user", "", "", "")

	if err := ensureAdminCredentials(); err != nil {
		t.Fatalf("ensure admin credentials: %v", err)
	}

	envFile, err := godotenv.Read(".env")
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	if envFile["USERNAME"] != "" || envFile["PASSWORD"] != "" {
		t.Fatal("expected plaintext credentials to be cleared")
	}

	hashedUsername, err := credentials.DecodeHash(envFile["HASHED_USERNAME"])
	if err != nil {
		t.Fatalf("decode hashed username: %v", err)
	}
	if !credentials.Matches(hashedUsername, "new-admin") {
		t.Fatal("expected username hash to match")
	}

	hashedPassword, err := credentials.DecodeHash(envFile["HASHED_PASSWORD"])
	if err != nil {
		t.Fatalf("decode hashed password: %v", err)
	}
	if !credentials.Matches(hashedPassword, "new-password") {
		t.Fatal("expected password hash to match")
	}
}

func TestEnsureAdminCredentialsAcceptsProcessEnvPair(t *testing.T) {
	chdirTemp(t)
	writeEnvFile(t, "APP_ENV=dev\n")
	setCredentialEnv(t, "env-admin", "env-password", "", "")

	if err := ensureAdminCredentials(); err != nil {
		t.Fatalf("ensure admin credentials: %v", err)
	}

	envFile, err := godotenv.Read(".env")
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	hashedUsername, err := credentials.DecodeHash(envFile["HASHED_USERNAME"])
	if err != nil {
		t.Fatalf("decode hashed username: %v", err)
	}
	if !credentials.Matches(hashedUsername, "env-admin") {
		t.Fatal("expected username hash to match")
	}
}

func TestEnsureAdminCredentialsIgnoresHostUsername(t *testing.T) {
	chdirTemp(t)
	writeEnvFile(t, "USERNAME=\nPASSWORD=\n")
	setCredentialEnv(t, "host-user", "", "", "")

	if err := ensureAdminCredentials(); !errors.Is(err, errCredentialsNotSet) {
		t.Fatalf("expected %v, got %v", errCredentialsNotSet, err)
	}
}

func TestEnsureAdminCredentialsKeepsExistingHashes(t *testing.T) {
	chdirTemp(t)

	hashedUsername := hashForTest(t, "admin")
	hashedPassword := hashForTest(t, "s3cret")
	writeEnvFile(t, "HASHED_USERNAME="+hashedUsername+"\nHASHED_PASSWORD="+hashedPassword+"\n")
	setCredentialEnv(t, "", "", hashedUsername, hashedPassword)

	if err := ensureAdminCredentials(); err != nil {
		t.Fatalf("ensure admin credentials: %v", err)
	}

	envFile, err := godotenv.Read(".env")
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if envFile["HASHED_USERNAME"] != hashedUsername || envFile["HASHED_PASSWORD"] != hashedPassword {
		t.Fatal("expected existing hashes to be left untouched")
	}
}

func TestEnsureAdminCredentialsRefusesToStart(t *testing.T) {
	emptyUsernameHash := hashForTest(t, "")
	emptyPasswordHash := hashForTest(t, "")
	validHash := hashForTest(t, "admin")

	cases := map[string]struct {
		username, password             string
		hashedUsername, hashedPassword string
		want                           error
	}{
		"nothing set": {
			want: errCredentialsNotSet,
		},
		"blank plaintext pair": {
			username: "   ", password: "\t\n",
			want: errCredentialsNotSet,
		},
		"username only": {
			username: "admin",
			want:     errCredentialsIncomplete,
		},
		"password only": {
			password: "s3cret",
			want:     errCredentialsIncomplete,
		},
		"blank password next to a real username": {
			username: "admin", password: "   ",
			want: errCredentialsIncomplete,
		},
		"username only next to existing hashes": {
			username:       "admin",
			hashedUsername: validHash, hashedPassword: validHash,
			want: errCredentialsIncomplete,
		},
		"half of the hashed pair": {
			hashedUsername: validHash,
			want:           errCredentialsNotSet,
		},
		"malformed hash": {
			hashedUsername: validHash, hashedPassword: "not-base64!!!",
			want: errCredentialsNotSet,
		},
		"hashes of empty credentials": {
			hashedUsername: emptyUsernameHash, hashedPassword: emptyPasswordHash,
			want: errCredentialsEmpty,
		},
		"hash of an empty password": {
			hashedUsername: validHash, hashedPassword: emptyPasswordHash,
			want: errCredentialsEmpty,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			before := "USERNAME=" + tc.username + "\nPASSWORD=" + tc.password +
				"\nHASHED_USERNAME=" + tc.hashedUsername + "\nHASHED_PASSWORD=" + tc.hashedPassword + "\n"
			writeEnvFile(t, before)
			setCredentialEnv(t, tc.username, tc.password, tc.hashedUsername, tc.hashedPassword)

			err := ensureAdminCredentials()
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}

			after, readErr := os.ReadFile(".env")
			if readErr != nil {
				t.Fatalf("read env file: %v", readErr)
			}
			if string(after) != before {
				t.Fatalf("expected a refused start to leave the .env file alone, got:\n%s", after)
			}
		})
	}
}

func chdirTemp(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
}

func writeEnvFile(t *testing.T, content string) {
	t.Helper()

	if err := os.WriteFile(".env", []byte(content), 0o666); err != nil {
		t.Fatalf("write env file: %v", err)
	}
}

func setCredentialEnv(t *testing.T, username, password, hashedUsername, hashedPassword string) {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("SESSION_DRIVER", "db")
	t.Setenv("PORT", "8999")
	t.Setenv("DSN", "umono.db")
	t.Setenv(credentials.UsernameEnvKey, username)
	t.Setenv(credentials.PasswordEnvKey, password)
	t.Setenv(credentials.HashedUsernameEnvKey, hashedUsername)
	t.Setenv(credentials.HashedPasswordEnvKey, hashedPassword)
}

func hashForTest(t *testing.T, value string) string {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash %q: %v", value, err)
	}

	return base64.StdEncoding.EncodeToString(hashed)
}

func TestDevRootFromEnv(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv("UMONO_THEME_DEV", "x")
		if err := os.Unsetenv("UMONO_THEME_DEV"); err != nil {
			t.Fatalf("unsetenv: %v", err)
		}

		root, err := devRootFromEnv()
		if root != "" {
			t.Fatalf("expected empty root, got %q", root)
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv("UMONO_THEME_DEV", "")

		_, err := devRootFromEnv()
		if !errors.Is(err, theme.ErrDevRoot) {
			t.Fatalf("expected %v, got %v", theme.ErrDevRoot, err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("UMONO_THEME_DEV", dir)

		root, err := devRootFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if root != dir {
			t.Fatalf("expected root %q, got %q", dir, root)
		}
	})
}

func TestRegistryURLFromEnv(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv("UMONO_REGISTRY", "x")
		if err := os.Unsetenv("UMONO_REGISTRY"); err != nil {
			t.Fatalf("unsetenv: %v", err)
		}

		url, err := registryURLFromEnv()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if url != theme.RegistryURL {
			t.Fatalf("expected %q, got %q", theme.RegistryURL, url)
		}
	})

	t.Run("mirror", func(t *testing.T) {
		t.Setenv("UMONO_REGISTRY", "https://registry.example.lan/")

		url, err := registryURLFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != "https://registry.example.lan" {
			t.Fatalf("expected %q, got %q", "https://registry.example.lan", url)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv("UMONO_REGISTRY", "")

		_, err := registryURLFromEnv()
		if !errors.Is(err, theme.ErrRegistryURL) {
			t.Fatalf("expected %v, got %v", theme.ErrRegistryURL, err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Setenv("UMONO_REGISTRY", "ftp://registry.example.lan")

		_, err := registryURLFromEnv()
		if !errors.Is(err, theme.ErrRegistryURL) {
			t.Fatalf("expected %v, got %v", theme.ErrRegistryURL, err)
		}
	})
}
