package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testUpdaterScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return updaterScript(
		"https://example.invalid/v1.2.3.4/OtherSync.msix",
		"Nimbo_hthxzjrt90bbe",
		filepath.Join(dir, "othersync-update.log"),
		"NimboSelfUpdate",
		filepath.Join(dir, "othersync-update-task.xml"),
		"Nimbo", "Nimbo", `nimbo-gui\.exe`, "nimbo-gui",
		filepath.Join(dir, "othersync-update.vbs"),
	)
}

// The updater checks the relaunched app is still running and starts it again
// if not (the 0.4.1.412 update relaunched an app that died a second later).
func TestUpdaterScriptRetriesRelaunch(t *testing.T) {
	s := testUpdaterScript(t)
	for _, want := range []string{
		"app running after relaunch",
		"app not running after relaunch; starting it again",
		"Get-Process -Name 'nimbo-gui'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("updater script is missing %q", want)
		}
	}
	// One initial launch plus at most two retries.
	if n := strings.Count(s, "explorer.exe 'shell:AppsFolder\\Nimbo_hthxzjrt90bbe!Nimbo'"); n != 2 {
		t.Errorf("explorer relaunch lines = %d, want 2 (first launch + the retry in the loop)", n)
	}
	// The watchdog runs after the install and before the task removes itself.
	install := strings.Index(s, "Add-AppxPackage")
	watch := strings.Index(s, "app running after relaunch")
	cleanup := strings.Index(s, "schtasks /delete")
	if !(install < watch && watch < cleanup) {
		t.Errorf("order install=%d watchdog=%d cleanup=%d, want install < watchdog < cleanup", install, watch, cleanup)
	}
}

// The whole script must parse: a syntax error would only show up mid-update,
// after the app has already been closed.
func TestUpdaterScriptParses(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("powershell.exe not available")
	}
	ps1 := filepath.Join(t.TempDir(), "updater.ps1")
	if err := os.WriteFile(ps1, []byte(testUpdaterScript(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	check := `$errs = $null; $tokens = $null; ` +
		`[System.Management.Automation.Language.Parser]::ParseFile('` + ps1 + `', [ref]$tokens, [ref]$errs) | Out-Null; ` +
		`$errs | ForEach-Object { $_.ToString() }`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", check).CombinedOutput()
	if err != nil {
		t.Fatalf("running the parser: %v: %s", err, out)
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		t.Fatalf("updater script has PowerShell syntax errors:\n%s", msg)
	}
}
