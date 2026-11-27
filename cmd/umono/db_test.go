package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/umono-cms/umono/internal/credentials"
	"github.com/umono-cms/umono/internal/repository"
	"github.com/umono-cms/umono/internal/theme"
)

// syncBuffer is a bytes.Buffer that the job service goroutine and the test
// can use at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The first minutes of a new project log no errors: GORM does not log the
// expected "record not found" results of startup, of the first requests and
// of the polls of the empty job queue.
func TestOpenDBLogsNoErrorsInTheFirstMinutesOfANewProject(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, kv := range []struct{ key, value string }{
		{credentials.HashedUsernameEnvKey, testAdminUsername},
		{credentials.HashedPasswordEnvKey, testAdminPassword},
	} {
		h, err := credentials.Hash(kv.value)
		if err != nil {
			t.Fatalf("hash %s: %v", kv.key, err)
		}
		t.Setenv(kv.key, credentials.Encode(h))
	}
	t.Setenv("SESSION_DRIVER", "")

	var out syncBuffer
	db, err := openDB("umono.db", &out)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	app, err := newApp(db, newTestSecret(t), "", theme.RegistryURL)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	for _, path := range []string{"/", "/admin/login", "/favicon.ico"} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
	}

	// The job service polls the empty queue once a minute.
	jobs := repository.NewJobRepository(db)
	for range 3 {
		if _, _, err := jobs.AcquireDue(context.Background(), time.Now()); err != nil {
			t.Fatalf("acquire due: %v", err)
		}
		if _, err := jobs.NextRunAt(context.Background()); err != nil {
			t.Fatalf("next run at: %v", err)
		}
	}

	if strings.Contains(out.String(), "record not found") {
		t.Fatalf("log has record not found errors:\n%s", out.String())
	}
}

// A real database error is still logged.
func TestOpenDBLogsDatabaseErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	var out syncBuffer
	db, err := openDB("umono.db", &out)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Exec("SELECT * FROM missing_table").Error; err == nil {
		t.Fatal("query a missing table: want an error")
	}

	if !strings.Contains(out.String(), "no such table: missing_table") {
		t.Fatalf("log = %q, want the database error", out.String())
	}
}
