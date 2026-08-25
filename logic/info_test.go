package logic

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// setFakeSysfs points the package-level sysCPUDir at a temporary directory
// mimicking /sys/devices/system/cpu/ for the duration of the test.
func setFakeSysfs(t *testing.T, totalCores int, onlineStates map[int]bool) string {
	t.Helper()
	return buildFakeSysfs(t, totalCores, func(i int) (string, bool) {
		if i == 0 {
			return "", false // cpu0 has no online file
		}
		v := "0"
		if onlineStates[i] {
			v = "1"
		}
		return v + "\n", true // kernel writes include a trailing newline
	})
}

// buildFakeSysfs creates cpuN directories using the supplied per-core
// online-file factory (value, present).
func buildFakeSysfs(t *testing.T, totalCores int, onlineFile func(i int) (string, bool)) string {
	t.Helper()

	old := sysCPUDir
	cpuDir := filepath.Join(t.TempDir(), "cpu")
	if err := os.MkdirAll(cpuDir, 0755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < totalCores; i++ {
		coreDir := filepath.Join(cpuDir, "cpu"+strconv.Itoa(i))
		if err := os.MkdirAll(coreDir, 0755); err != nil {
			t.Fatal(err)
		}
		if v, ok := onlineFile(i); ok {
			if err := os.WriteFile(filepath.Join(coreDir, "online"), []byte(v), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	sysCPUDir = cpuDir
	t.Cleanup(func() { sysCPUDir = old })
	return cpuDir
}

// setFakeProcCPUinfo points procCPUInfoPath at a temporary file.
func setFakeProcCPUinfo(t *testing.T, content string) {
	t.Helper()

	old := procCPUInfoPath
	path := filepath.Join(t.TempDir(), "cpuinfo")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	procCPUInfoPath = path
	t.Cleanup(func() { procCPUInfoPath = old })
}

func readOnline(t *testing.T, coreID int) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sysCPUDir, "cpu"+strconv.Itoa(coreID), "online"))
	if err != nil {
		t.Fatalf("cpu%d online: %v", coreID, err)
	}
	return strings.TrimSpace(string(data))
}

func TestGetCPUModel_ParsesModelName(t *testing.T) {
	setFakeProcCPUinfo(t, "processor\t: 0\nmodel name\t: Test CPU Model @ 3.2GHz\nflags\t\t: fpu\n")
	model, err := GetCPUModel()
	if err != nil {
		t.Fatal(err)
	}
	if model != "Test CPU Model @ 3.2GHz" {
		t.Errorf("got %q", model)
	}
}

func TestGetCPUModel_MissingReturnsUnknown(t *testing.T) {
	setFakeProcCPUinfo(t, "processor\t: 0\ncache size\t: 512 KB\n")
	model, err := GetCPUModel()
	if err != nil {
		t.Fatal(err)
	}
	if model != "Unknown" {
		t.Errorf("expected Unknown, got %q", model)
	}
}

func TestListAllCPUCores_CountsOnlyNumericCPUDirs(t *testing.T) {
	cpuDir := setFakeSysfs(t, 4, map[int]bool{1: true})

	// Decoys that must not be counted.
	for _, name := range []string{"cpufreq", "cpuidle", "cpufoo", "cpu+5", "cpu-3"} {
		if err := os.MkdirAll(filepath.Join(cpuDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"online", "present", "cpu9", "cpulist"} {
		if err := os.WriteFile(filepath.Join(cpuDir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ListAllCPUCores()
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Errorf("expected 4 cores, got %d", got)
	}
}

func TestParseCoreSuffix(t *testing.T) {
	cases := []struct {
		suffix string
		wantID int
		wantOK bool
	}{
		{"0", 0, true},
		{"12", 12, true},
		{"007", 7, true},
		{"", 0, false},
		{"+5", 0, false},
		{"-3", 0, false},
		{"1a", 0, false},
		{"a2", 0, false},
		{"99999999999999999999999", 0, false}, // overflow
	}
	for _, c := range cases {
		id, ok := parseCoreSuffix(c.suffix)
		if ok != c.wantOK || (ok && id != c.wantID) {
			t.Errorf("parseCoreSuffix(%q) = (%d, %v), want (%d, %v)", c.suffix, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestListAllCPUCores_MissingSysfsErrors(t *testing.T) {
	old := sysCPUDir
	sysCPUDir = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { sysCPUDir = old })

	if _, err := ListAllCPUCores(); err == nil {
		t.Error("expected error for missing sysfs directory")
	}
}

func TestGetCoreStates_KernelFormatWithNewline(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: true, 2: false, 3: true})

	states, err := GetCoreStates()
	if err != nil {
		t.Fatal(err)
	}

	expected := []bool{true, true, false, true} // cpu0 implicit on
	for i, want := range expected {
		if states[i] != want {
			t.Errorf("cpu%d: expected %v, got %v", i, want, states[i])
		}
	}
}

func TestGetCoreStates_UnreadableCoreIsAnError(t *testing.T) {
	cpuDir := setFakeSysfs(t, 4, map[int]bool{1: true, 2: false, 3: true})

	// Make cpu1's online file unreadable as a file (a directory in its place).
	if err := os.Remove(filepath.Join(cpuDir, "cpu1", "online")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cpuDir, "cpu1", "online"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := GetCoreStates()
	if err == nil {
		t.Fatal("expected error for unreadable core state")
	}
	if !strings.Contains(err.Error(), "cpu1") {
		t.Errorf("error should name the offending core, got: %v", err)
	}
}

func TestGetActiveCores_MatchesStates(t *testing.T) {
	setFakeSysfs(t, 4, map[int]bool{1: false, 2: false, 3: true})

	active, err := GetActiveCores()
	if err != nil {
		t.Fatal(err)
	}
	if active != 2 { // cpu0 + cpu3
		t.Errorf("expected 2 active cores, got %d", active)
	}
}

func TestGetInfo_ConsistentSnapshot(t *testing.T) {
	setFakeProcCPUinfo(t, "model name\t: Snapshot CPU\n")
	setFakeSysfs(t, 6, map[int]bool{1: true, 2: true, 3: false, 4: false, 5: false})

	info, err := GetInfo()
	if err != nil {
		t.Fatal(err)
	}

	if info.Model != "Snapshot CPU" {
		t.Errorf("model: got %q", info.Model)
	}
	if len(info.CoreIDs) != info.TotalCores || len(info.CoreStates) != info.TotalCores {
		t.Fatalf("ids/states length mismatch: %d ids, %d states, %d total", len(info.CoreIDs), len(info.CoreStates), info.TotalCores)
	}
	for k := range info.CoreIDs {
		if k > 0 && info.CoreIDs[k] <= info.CoreIDs[k-1] {
			t.Errorf("core IDs not ascending: %v", info.CoreIDs)
		}
	}
	if info.ActiveCores != countOnline(info.CoreStates) {
		t.Errorf("active %d inconsistent with states %v", info.ActiveCores, info.CoreStates)
	}
	if info.TotalCores != 6 || info.ActiveCores != 3 {
		t.Errorf("expected 6 total / 3 active, got %d / %d", info.TotalCores, info.ActiveCores)
	}
}

func TestScanCoreStates_SparseCoreIDs(t *testing.T) {
	cpuDir := setFakeSysfs(t, 4, map[int]bool{1: true, 2: false, 3: true})

	// Remove cpu1 entirely to simulate non-contiguous core numbering.
	if err := os.RemoveAll(filepath.Join(cpuDir, "cpu1")); err != nil {
		t.Fatal(err)
	}

	ids, states, err := scanCoreStates()
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int{0, 2, 3}
	wantStates := []bool{true, false, true}
	for i := range wantIDs {
		if ids[i] != wantIDs[i] || states[i] != wantStates[i] {
			t.Fatalf("got ids=%v states=%v, want ids=%v states=%v", ids, states, wantIDs, wantStates)
		}
	}
}

func TestCountOnline(t *testing.T) {
	cases := []struct {
		states []bool
		want   int
	}{
		{[]bool{}, 0},
		{[]bool{false, false}, 0},
		{[]bool{true, false, true}, 2},
	}
	for _, c := range cases {
		if got := countOnline(c.states); got != c.want {
			t.Errorf("countOnline(%v) = %d, want %d", c.states, got, c.want)
		}
	}
}

func TestAllCPU_ReturnsRawContent(t *testing.T) {
	setFakeProcCPUinfo(t, "RAW CONTENT\nline two\n")
	raw, err := AllCPU()
	if err != nil {
		t.Fatal(err)
	}
	if raw != "RAW CONTENT\nline two\n" {
		t.Errorf("got %q", raw)
	}
}
