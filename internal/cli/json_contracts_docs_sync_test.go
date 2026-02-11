package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestJSONContractsDocsStayInSync(t *testing.T) {
	t.Parallel()

	type fixturePair struct {
		name   string
		golden string
		docs   string
	}

	pairs := []fixturePair{
		{name: "entry_add", golden: "entry_add.golden.json", docs: "entry-add.json"},
		{name: "entry_update", golden: "entry_update.golden.json", docs: "entry-update.json"},
		{name: "cap_set", golden: "cap_set.golden.json", docs: "cap-set.json"},
		{name: "cap_show", golden: "cap_show.golden.json", docs: "cap-show.json"},
		{name: "cap_history", golden: "cap_history.golden.json", docs: "cap-history.json"},
		{name: "report_monthly", golden: "report_monthly.golden.json", docs: "report-monthly.json"},
		{name: "report_range", golden: "report_range.golden.json", docs: "report-range.json"},
		{name: "report_bimonthly", golden: "report_bimonthly.golden.json", docs: "report-bimonthly.json"},
		{name: "report_quarterly", golden: "report_quarterly.golden.json", docs: "report-quarterly.json"},
		{name: "balance_show", golden: "balance_show.golden.json", docs: "balance-show.json"},
		{name: "data_export_entries", golden: "data_export_entries.golden.json", docs: "data-export.json"},
		{name: "data_export_report", golden: "data_export_report.golden.json", docs: "data-export-report.json"},
		{name: "setup_init", golden: "setup_init.golden.json", docs: "setup-init.json"},
	}

	for _, pair := range pairs {
		pair := pair
		t.Run(pair.name, func(t *testing.T) {
			t.Parallel()

			goldenPath := filepath.Join(jsonContractsGoldenDir(t), pair.golden)
			docsPath := filepath.Join(docsContractsDir(t), pair.docs)

			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden fixture %q: %v", goldenPath, err)
			}

			docs, err := os.ReadFile(docsPath)
			if err != nil {
				t.Fatalf("read docs contract %q: %v", docsPath, err)
			}

			goldenCanonical := canonicalJSONContract(t, golden)
			docsCanonical := canonicalJSONContract(t, docs)

			if !bytes.Equal(goldenCanonical, docsCanonical) {
				t.Fatalf("docs contract mismatch for %s\ndocs=%s\ngolden=%s\nexpected:\n%s\nactual:\n%s", pair.name, docsPath, goldenPath, goldenCanonical, docsCanonical)
			}
		})
	}
}

func canonicalJSONContract(t *testing.T, raw []byte) []byte {
	t.Helper()

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal json fixture: %v raw=%s", err, string(raw))
	}

	return marshalNormalizedJSON(t, normalizeJSONContractValue("", payload))
}

func docsContractsDir(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "docs", "contracts")
}
