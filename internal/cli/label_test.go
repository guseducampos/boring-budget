package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"boring-budget/internal/cli/output"
	sqlitestore "boring-budget/internal/store/sqlite"
)

func TestLabelCommandJSONLifecycle(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addPayload := executeLabelCmdJSON(t, db, []string{"add", "groceries"})
	if ok, _ := addPayload["ok"].(bool); !ok {
		t.Fatalf("expected add ok=true, got payload=%v", addPayload)
	}

	addData := mustMap(t, addPayload["data"])
	addedLabel := mustMap(t, addData["label"])
	labelID := int64(addedLabel["id"].(float64))
	if addedLabel["name"].(string) != "groceries" {
		t.Fatalf("expected label name groceries, got %v", addedLabel["name"])
	}

	listPayload := executeLabelCmdJSON(t, db, []string{"list"})
	listData := mustMap(t, listPayload["data"])
	if int(listData["count"].(float64)) != 1 {
		t.Fatalf("expected list count 1, got %v", listData["count"])
	}

	renamePayload := executeLabelCmdJSON(t, db, []string{"rename", "1", "food"})
	renameData := mustMap(t, renamePayload["data"])
	renamedLabel := mustMap(t, renameData["label"])
	if renamedLabel["name"].(string) != "food" {
		t.Fatalf("expected renamed label name food, got %v", renamedLabel["name"])
	}

	deletePayload := executeLabelCmdJSON(t, db, []string{"delete", "1"})
	deleteData := mustMap(t, deletePayload["data"])
	deleted := mustMap(t, deleteData["deleted"])
	if int64(deleted["label_id"].(float64)) != labelID {
		t.Fatalf("expected deleted label_id %d, got %v", labelID, deleted["label_id"])
	}

	finalListPayload := executeLabelCmdJSON(t, db, []string{"list"})
	finalListData := mustMap(t, finalListPayload["data"])
	if int(finalListData["count"].(float64)) != 0 {
		t.Fatalf("expected final list count 0, got %v", finalListData["count"])
	}
}

func TestLabelCommandJSONInvalidID(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeLabelCmdJSON(t, db, []string{"delete", "abc"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false for invalid id payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestLabelCommandHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	out := executeLabelCmdRaw(t, db, output.FormatHuman, []string{"add", "groceries"})
	if !strings.Contains(out, "[OK] boring-budget") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"name\": \"groceries\"") {
		t.Fatalf("expected human output to include label name, got %q", out)
	}
}

func executeLabelCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeLabelCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal json payload: %v, raw=%s", err, raw)
	}
	return payload
}

func executeLabelCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewLabelCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute label cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()

	mapped, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return mapped
}

func newCLITestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cli_test.db")

	db, err := sqlitestore.OpenAndMigrate(ctx, dbPath, cliMigrationsPath(t))
	if err != nil {
		t.Fatalf("open and migrate cli test db: %v", err)
	}

	return db
}

func cliMigrationsPath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations")
}
