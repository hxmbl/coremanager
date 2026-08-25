package logic

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// makeUnwritableCore strips write permission from cpuN's online file so
// reads still succeed but any write attempt fails with EACCES, simulating a
// stubborn kernel mid-operation. Skipped when tests run as root.
func makeUnwritableCore(t *testing.T, coreID int) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("write-permission simulation does not apply to root")
	}
	path := filepath.Join(sysCPUDir, "cpu"+strconv.Itoa(coreID), "online")
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
}

func TestEnableAll_EnablesOnlyOfflineCores(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: true, 2: false, 3: false})

	n, err := EnableAll(false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 cores changed, got %d", n)
	}
	for i := 1; i <= 3; i++ {
		if got := readOnline(t, i); got != "1" {
			t.Errorf("cpu%d: expected online, got %q", i, got)
		}
	}
}

func TestDisableAll_DisablesOnlyOnlineCores(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: false, 2: true, 3: false})

	n, err := DisableAll(false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 core changed, got %d", n)
	}
	if got := readOnline(t, 2); got != "0" {
		t.Errorf("cpu2: expected offline, got %q", got)
	}
}

func TestEnableAll_NoSecondaryCoresIsNoOp(t *testing.T) {
	setFakeSysfs(t, 1, nil) // cpu0 only

	n, err := EnableAll(false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 changes on single-core system, got %d", n)
	}

	if n, err := DisableAll(false); err != nil || n != 0 {
		t.Errorf("DisableAll: got (%d, %v)", n, err)
	}
}

func TestEnable_EnablesLowestOfflineFirst(t *testing.T) {
	setFakeSysfs(t, 5, map[int]bool{1: true, 2: false, 3: true, 4: false})

	n, err := Enable(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 core enabled, got %d", n)
	}
	if got := readOnline(t, 2); got != "1" {
		t.Errorf("cpu2 should have been enabled first, got %q", got)
	}
	if got := readOnline(t, 4); got != "0" {
		t.Errorf("cpu4 should be untouched, got %q", got)
	}

	// Second call must move on to the next offline core.
	if n, err = Enable(1, false); err != nil || n != 1 {
		t.Fatalf("second enable: got (%d, %v)", n, err)
	}
	if got := readOnline(t, 4); got != "1" {
		t.Errorf("cpu4: expected online, got %q", got)
	}
}

func TestEnable_ErrorsWhenInsufficientCores(t *testing.T) {
	states := map[int]bool{1: false, 2: false, 3: false}
	setFakeSysfs(t, 4, states)

	if _, err := Enable(4, false); err == nil {
		t.Fatal("expected error enabling more cores than are disabled")
	} else if !strings.Contains(err.Error(), "Only 3 disabled") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Nothing may have been written.
	for i := 1; i <= 3; i++ {
		if got := readOnline(t, i); got != "0" {
			t.Errorf("cpu%d: expected untouched, got %q", i, got)
		}
	}
}

func TestEnable_RejectsNonPositiveCount(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: false})
	for _, n := range []int{0, -1} {
		if _, err := Enable(n, false); err == nil {
			t.Errorf("Enable(%d): expected error", n)
		}
		if _, err := Disable(n, false); err == nil {
			t.Errorf("Disable(%d): expected error", n)
		}
	}
}

func TestDisable_RespectsCPU0AndGuard(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: true, 2: true, 3: true}) // all secondary on

	n, err := Disable(3, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("expected 3 disabled, got %d", n)
	}
	for i := 1; i <= 3; i++ {
		if got := readOnline(t, i); got != "0" {
			t.Errorf("cpu%d: expected offline, got %q", i, got)
		}
	}
	if _, statErr := os.Stat(filepath.Join(sysCPUDir, "cpu0", "online")); !os.IsNotExist(statErr) {
		t.Errorf("cpu0 must never get an online file, stat err: %v", statErr)
	}

	// Only cpu0 remains; further disables must fail.
	if _, err := Disable(1, false); err == nil {
		t.Error("expected error disabling below 1 active core")
	}
}

func TestDisable_PartialFailureReportsProgress(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: true, 2: true, 3: true})
	makeUnwritableCore(t, 2) // cpu2 write will fail

	n, err := Disable(2, false)
	if err == nil {
		t.Fatal("expected error from unwritable core")
	}
	if n != 1 {
		t.Errorf("expected 1 successful disable before failure, got %d", n)
	}
	if !strings.Contains(err.Error(), "disabled 1 of 2") {
		t.Errorf("error should report partial progress, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cpu2") {
		t.Errorf("error should name failing core, got: %v", err)
	}

	if got := readOnline(t, 1); got != "0" {
		t.Errorf("cpu1 should be offline after partial run, got %q", got)
	}
	if got := readOnline(t, 3); got != "1" {
		t.Errorf("cpu3 should remain untouched, got %q", got)
	}
}

func TestEnableDisable_SparseCoreIDs(t *testing.T) {
	cpuDir := setFakeSysfs(t, 4, map[int]bool{1: true, 2: false, 3: false})
	if err := os.RemoveAll(filepath.Join(cpuDir, "cpu1")); err != nil {
		t.Fatal(err)
	}
	// Remaining secondary cores: cpu2 (offline), cpu3 (offline).

	n, err := Enable(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 enabled, got %d", n)
	}
	if got := readOnline(t, 2); got != "1" {
		t.Errorf("cpu2 should be the first enabled core, got %q", got)
	}

	n, err = Disable(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 disabled, got %d", n)
	}
	if got := readOnline(t, 2); got != "0" {
		t.Errorf("cpu2 should be disabled again, got %q", got)
	}
	if got := readOnline(t, 3); got != "0" {
		t.Errorf("cpu3 should remain untouched, got %q", got)
	}
}

func TestSetCoreOnline_WritesAndVerifies(t *testing.T) {
	setFakeSysfs(t, 3, map[int]bool{1: false})

	if err := setCoreOnline(1, true); err != nil {
		t.Fatal(err)
	}
	if got := readOnline(t, 1); got != "1" {
		t.Errorf("expected \"1\", got %q", got)
	}

	if err := setCoreOnline(1, false); err != nil {
		t.Fatal(err)
	}
	if got := readOnline(t, 1); got != "0" {
		t.Errorf("expected \"0\", got %q", got)
	}
}

func TestSetCoreOnline_VerificationCatchesStubbornCore(t *testing.T) {
	setFakeSysfs(t, 3, map[int]bool{1: false})

	// Simulate a kernel that ignores the write: the file still reports "1"
	// even though we asked for offline. verifyCoreState must catch it.
	if err := os.WriteFile(filepath.Join(sysCPUDir, "cpu1", "online"), []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := verifyCoreState(1, false)
	if err == nil {
		t.Fatal("expected verification error for core stuck online")
	}
	if !strings.Contains(err.Error(), "did not take effect") {
		t.Errorf("unexpected error: %v", err)
	}
}
