package cli

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildManagedScheduleCronEntryIncludesExpectedCommand(t *testing.T) {
	t.Parallel()

	entry, marker := buildManagedScheduleCronEntry("/usr/local/bin/boring-budget", "/tmp/boring-budget.db")
	if !strings.Contains(entry, "schedule run") {
		t.Fatalf("expected schedule run command in cron entry: %q", entry)
	}
	if !strings.Contains(entry, "--through-date") {
		t.Fatalf("expected through-date flag in cron entry: %q", entry)
	}
	if !strings.Contains(entry, marker) {
		t.Fatalf("expected marker in cron entry marker=%q entry=%q", marker, entry)
	}
	if !strings.Contains(entry, "\\%Y-\\%m-\\%d") {
		t.Fatalf("expected escaped cron date format in cron entry: %q", entry)
	}
}

func TestUpsertManagedCronEntryAddsOnlyOnce(t *testing.T) {
	t.Parallel()

	entry, marker := buildManagedScheduleCronEntry("/usr/local/bin/boring-budget", "/tmp/boring-budget.db")
	updated, changed := upsertManagedCronEntry("", marker, entry)
	if !changed {
		t.Fatalf("expected cron entry to be added")
	}
	if strings.Count(updated, marker) != 1 {
		t.Fatalf("expected one marker occurrence, got content: %q", updated)
	}

	updatedAgain, changedAgain := upsertManagedCronEntry(updated, marker, entry)
	if changedAgain {
		t.Fatalf("expected no-op when marker already exists")
	}
	if updatedAgain != updated {
		t.Fatalf("expected unchanged cron content")
	}
}

func TestEnsureScheduleCronRegistrationWritesCrontabOnSupportedPlatforms(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("cron registration is supported on Linux and macOS only")
	}

	originalList := listCrontabFn
	originalWrite := writeCrontabFn
	t.Cleanup(func() {
		listCrontabFn = originalList
		writeCrontabFn = originalWrite
	})

	listCrontabFn = func() (string, error) { return "", nil }

	wrote := ""
	writeCrontabFn = func(content string) error {
		wrote = content
		return nil
	}

	opts := &RootOptions{DBPath: filepath.Join(t.TempDir(), "budget.db")}
	if err := ensureScheduleCronRegistration(opts); err != nil {
		t.Fatalf("ensure schedule cron registration: %v", err)
	}

	if strings.TrimSpace(wrote) == "" {
		t.Fatalf("expected non-empty crontab content")
	}
	if !strings.Contains(wrote, "schedule run") {
		t.Fatalf("expected schedule run cron entry, got: %q", wrote)
	}
}
