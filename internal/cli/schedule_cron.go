package cli

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	listCrontabFn  = listUserCrontab
	writeCrontabFn = writeUserCrontab
)

func ensureScheduleCronRegistration(opts *RootOptions) error {
	if runtime.GOOS != "linux" || opts == nil {
		return nil
	}

	dbPath := strings.TrimSpace(opts.DBPath)
	if dbPath == "" {
		return nil
	}

	absoluteDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	entry, marker := buildManagedScheduleCronEntry(execPath, absoluteDBPath)
	current, err := listCrontabFn()
	if err != nil {
		return err
	}

	updated, changed := upsertManagedCronEntry(current, marker, entry)
	if !changed {
		return nil
	}

	return writeCrontabFn(updated)
}

func buildManagedScheduleCronEntry(execPath, absoluteDBPath string) (entry string, marker string) {
	hash := sha1.Sum([]byte(strings.TrimSpace(absoluteDBPath)))
	key := hex.EncodeToString(hash[:])[:12]
	marker = fmt.Sprintf("# boring-budget:schedule:%s", key)
	lockPath := fmt.Sprintf("/tmp/boring-budget-schedule-%s.lock", key)
	logPath := fmt.Sprintf("/tmp/boring-budget-schedule-%s.log", key)

	entry = fmt.Sprintf(
		"0 * * * * /usr/bin/flock -n %s %s --db-path %s schedule run --through-date \"$(/bin/date -u +\\%%Y-\\%%m-\\%%d)\" --output json >> %s 2>&1 %s",
		shellQuote(lockPath),
		shellQuote(execPath),
		shellQuote(absoluteDBPath),
		shellQuote(logPath),
		marker,
	)
	return entry, marker
}

func upsertManagedCronEntry(current, marker, entry string) (updated string, changed bool) {
	lines := strings.Split(strings.ReplaceAll(current, "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.Contains(line, marker) {
			return current, false
		}
	}

	trimmed := strings.TrimRight(current, "\n")
	if trimmed == "" {
		return entry + "\n", true
	}
	return trimmed + "\n" + entry + "\n", true
}

func listUserCrontab() (string, error) {
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), nil
	}

	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "no crontab for") {
		return "", nil
	}

	return "", fmt.Errorf("read crontab: %w: %s", err, strings.TrimSpace(string(output)))
}

func writeUserCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write crontab: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	replacer := strings.NewReplacer(`'`, `'"'"'`)
	return "'" + replacer.Replace(value) + "'"
}
