package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"boring-budget/internal/cli/output"
)

func TestBankAccountCommandJSONLifecycle(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addOne := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "Main Checking",
		"--last4", "1111",
	})
	if ok, _ := addOne["ok"].(bool); !ok {
		t.Fatalf("expected add one ok=true payload=%v", addOne)
	}
	addOneData := mustMap(t, addOne["data"])
	first := mustMap(t, addOneData["bank_account"])
	firstID := int64(first["id"].(float64))

	addTwo := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "Savings",
		"--last4", "2222",
	})
	if ok, _ := addTwo["ok"].(bool); !ok {
		t.Fatalf("expected add two ok=true payload=%v", addTwo)
	}
	addTwoData := mustMap(t, addTwo["data"])
	second := mustMap(t, addTwoData["bank_account"])
	secondID := int64(second["id"].(float64))

	listed := executeBankAccountCmdJSON(t, db, []string{"list"})
	if ok, _ := listed["ok"].(bool); !ok {
		t.Fatalf("expected list ok=true payload=%v", listed)
	}
	listData := mustMap(t, listed["data"])
	if int(listData["count"].(float64)) != 2 {
		t.Fatalf("expected 2 bank accounts, got %v", listData["count"])
	}
	accounts := mustAnySlice(t, listData["bank_accounts"])
	if len(accounts) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(accounts))
	}
	firstListed := mustMap(t, accounts[0])
	if int64(firstListed["id"].(float64)) != firstID {
		t.Fatalf("expected deterministic order with first account id=%d, got %v", firstID, firstListed["id"])
	}

	updated := executeBankAccountCmdJSON(t, db, []string{
		"update", strconv.FormatInt(secondID, 10),
		"--alias", "Emergency Fund",
		"--last4", "3333",
	})
	if ok, _ := updated["ok"].(bool); !ok {
		t.Fatalf("expected update ok=true payload=%v", updated)
	}

	lookupLast4 := executeBankAccountCmdJSON(t, db, []string{"list", "--lookup", "3333"})
	lookupData := mustMap(t, lookupLast4["data"])
	if int(lookupData["count"].(float64)) != 1 {
		t.Fatalf("expected 1 account by last4 lookup, got %v", lookupData["count"])
	}
	lookupRows := mustAnySlice(t, lookupData["bank_accounts"])
	lookupAccount := mustMap(t, lookupRows[0])
	if int64(lookupAccount["id"].(float64)) != secondID {
		t.Fatalf("expected lookup result id=%d, got %v", secondID, lookupAccount["id"])
	}

	deleted := executeBankAccountCmdJSON(t, db, []string{"delete", strconv.FormatInt(secondID, 10)})
	if ok, _ := deleted["ok"].(bool); !ok {
		t.Fatalf("expected delete ok=true payload=%v", deleted)
	}

	activeOnly := executeBankAccountCmdJSON(t, db, []string{"list"})
	activeData := mustMap(t, activeOnly["data"])
	if int(activeData["count"].(float64)) != 1 {
		t.Fatalf("expected 1 active bank account, got %v", activeData["count"])
	}

	withDeleted := executeBankAccountCmdJSON(t, db, []string{"list", "--include-deleted"})
	withDeletedData := mustMap(t, withDeleted["data"])
	if int(withDeletedData["count"].(float64)) != 2 {
		t.Fatalf("expected 2 accounts with include-deleted, got %v", withDeletedData["count"])
	}
	withDeletedRows := mustAnySlice(t, withDeletedData["bank_accounts"])
	deletedFound := false
	for _, row := range withDeletedRows {
		item := mustMap(t, row)
		if int64(item["id"].(float64)) == secondID {
			if _, ok := item["deleted_at_utc"].(string); !ok {
				t.Fatalf("expected deleted_at_utc string for deleted account, got %v", item["deleted_at_utc"])
			}
			deletedFound = true
		}
	}
	if !deletedFound {
		t.Fatalf("deleted account id=%d not found in include-deleted list", secondID)
	}

	lookupDeleted := executeBankAccountCmdJSON(t, db, []string{"list", "--lookup", "Emergency", "--include-deleted"})
	lookupDeletedData := mustMap(t, lookupDeleted["data"])
	if int(lookupDeletedData["count"].(float64)) != 1 {
		t.Fatalf("expected deleted lookup count 1, got %v", lookupDeletedData["count"])
	}
}

func TestBankAccountCommandJSONValidation(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	missingAlias := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--last4", "1111",
	})
	if ok, _ := missingAlias["ok"].(bool); ok {
		t.Fatalf("expected missing alias to fail payload=%v", missingAlias)
	}
	errPayload := mustMap(t, missingAlias["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}

	add := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "Main",
		"--last4", "1111",
	})
	if ok, _ := add["ok"].(bool); !ok {
		t.Fatalf("expected initial add success payload=%v", add)
	}
	addData := mustMap(t, add["data"])
	account := mustMap(t, addData["bank_account"])
	accountID := int64(account["id"].(float64))

	noUpdateFields := executeBankAccountCmdJSON(t, db, []string{"update", strconv.FormatInt(accountID, 10)})
	if ok, _ := noUpdateFields["ok"].(bool); ok {
		t.Fatalf("expected update without fields to fail payload=%v", noUpdateFields)
	}
	errPayload = mustMap(t, noUpdateFields["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}

	emptyLookup := executeBankAccountCmdJSON(t, db, []string{"list", "--lookup", "   "})
	if ok, _ := emptyLookup["ok"].(bool); !ok {
		t.Fatalf("expected empty lookup to be treated as unfiltered list payload=%v", emptyLookup)
	}
}

func executeBankAccountCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeBankAccountCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal bank-account payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeBankAccountCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewBankAccountCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute bank-account cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}
