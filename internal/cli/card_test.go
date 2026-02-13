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

func TestCardCommandJSONLifecycleAndDebt(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addPayload := executeCardCmdJSON(t, db, []string{
		"add",
		"--nickname", "Main Credit",
		"--description", "Primary credit card",
		"--last4", "1234",
		"--brand", "visa",
		"--card-type", "credit",
		"--due-day", "15",
	})
	if ok, _ := addPayload["ok"].(bool); !ok {
		t.Fatalf("expected add card ok=true payload=%v", addPayload)
	}

	addData := mustMap(t, addPayload["data"])
	card := mustMap(t, addData["card"])
	cardID := int64(card["id"].(float64))
	if card["card_type"].(string) != "credit" {
		t.Fatalf("expected credit card type, got %v", card["card_type"])
	}

	listPayload := executeCardCmdJSON(t, db, []string{"list"})
	if ok, _ := listPayload["ok"].(bool); !ok {
		t.Fatalf("expected list card ok=true payload=%v", listPayload)
	}
	listData := mustMap(t, listPayload["data"])
	if int(listData["count"].(float64)) != 1 {
		t.Fatalf("expected 1 card in list, got %v", listData["count"])
	}

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount", "20.00",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--payment-method", "card",
		"--card-id", strconv.FormatInt(cardID, 10),
	}))

	debtPayload := executeCardCmdJSON(t, db, []string{"debt", "show", "--card-id", strconv.FormatInt(cardID, 10)})
	if ok, _ := debtPayload["ok"].(bool); !ok {
		t.Fatalf("expected debt show ok=true payload=%v", debtPayload)
	}
	debtData := mustMap(t, debtPayload["data"])
	debt := mustMap(t, debtData["debt"])
	buckets := mustAnySlice(t, debt["buckets"])
	if len(buckets) != 1 {
		t.Fatalf("expected 1 debt bucket, got %d (%v)", len(buckets), buckets)
	}
	bucket := mustMap(t, buckets[0])
	if int64(bucket["balance_minor_signed"].(float64)) != 2000 {
		t.Fatalf("expected debt balance 2000, got %v", bucket["balance_minor_signed"])
	}
	if bucket["state"].(string) != "owes" {
		t.Fatalf("expected debt state owes, got %v", bucket["state"])
	}

	paymentPayload := executeCardCmdJSON(t, db, []string{
		"payment", "add",
		"--card-id", strconv.FormatInt(cardID, 10),
		"--amount", "5.00",
		"--currency", "USD",
	})
	if ok, _ := paymentPayload["ok"].(bool); !ok {
		t.Fatalf("expected payment add ok=true payload=%v", paymentPayload)
	}

	afterPaymentPayload := executeCardCmdJSON(t, db, []string{"debt", "show", "--card-id", strconv.FormatInt(cardID, 10)})
	if ok, _ := afterPaymentPayload["ok"].(bool); !ok {
		t.Fatalf("expected debt show after payment ok=true payload=%v", afterPaymentPayload)
	}
	afterPaymentData := mustMap(t, afterPaymentPayload["data"])
	afterDebt := mustMap(t, afterPaymentData["debt"])
	afterBuckets := mustAnySlice(t, afterDebt["buckets"])
	afterBucket := mustMap(t, afterBuckets[0])
	if int64(afterBucket["balance_minor_signed"].(float64)) != 1500 {
		t.Fatalf("expected debt balance 1500 after payment, got %v", afterBucket["balance_minor_signed"])
	}

	duePayload := executeCardCmdJSON(t, db, []string{"due", "show", "--card-id", strconv.FormatInt(cardID, 10), "--as-of", "2026-02-10"})
	if ok, _ := duePayload["ok"].(bool); !ok {
		t.Fatalf("expected due show ok=true payload=%v", duePayload)
	}
	dueData := mustMap(t, duePayload["data"])
	due := mustMap(t, dueData["due"])
	if due["due_day"].(float64) != 15 {
		t.Fatalf("expected due day 15, got %v", due["due_day"])
	}
	if !strings.HasPrefix(due["next_due_date_utc"].(string), "2026-02-15T") {
		t.Fatalf("expected next due date in 2026-02-15, got %v", due["next_due_date_utc"])
	}
}

func TestCardCommandJSONValidatesCardRules(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	invalidDueForDebit := executeCardCmdJSON(t, db, []string{
		"add",
		"--nickname", "Bad Debit",
		"--last4", "0001",
		"--brand", "visa",
		"--card-type", "debit",
		"--due-day", "10",
	})
	if ok, _ := invalidDueForDebit["ok"].(bool); ok {
		t.Fatalf("expected invalid debit due-day payload, got %v", invalidDueForDebit)
	}
	errPayload := mustMap(t, invalidDueForDebit["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}

	missingAmount := executeCardCmdJSON(t, db, []string{
		"payment", "add",
		"--card-id", "1",
	})
	if ok, _ := missingAmount["ok"].(bool); ok {
		t.Fatalf("expected missing amount payload, got %v", missingAmount)
	}
	errPayload = mustMap(t, missingAmount["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT for payment add, got %v", errPayload["code"])
	}
}

func executeCardCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeCardCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal card payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeCardCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, Timezone: "UTC", db: db}
	cmd := NewCardCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute card cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}
