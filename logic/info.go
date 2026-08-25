// Package logic implements CoreManager's CPU introspection and core
// online/offline control against Linux sysfs and procfs.
package logic

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Base paths, overridable in tests.
var (
	procCPUInfoPath = "/proc/cpuinfo"
	sysCPUDir       = "/sys/devices/system/cpu"
)

// CPUInfo holds information about the system's CPU. CoreStates[i] describes
// the core with ID CoreIDs[i].
type CPUInfo struct {
	Model       string
	TotalCores  int
	ActiveCores int
	CoreIDs     []int
	CoreStates  []bool
}

// GetCPUModel reads /proc/cpuinfo to get the CPU model name.
func GetCPUModel() (string, error) {
	f, err := os.Open(procCPUInfoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", procCPUInfoPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read %s: %w", procCPUInfoPath, err)
	}
	return "Unknown", nil
}

// parseCoreSuffix converts the suffix of a "cpuN" directory name into a core
// ID, accepting plain decimal digits only.
func parseCoreSuffix(suffix string) (int, bool) {
	if suffix == "" {
		return 0, false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return id, true
}

// listCPUCores returns the numerically sorted IDs of all cpuN directories
// found in the sysfs CPU directory. Non-directory entries (e.g. "online")
// and non-numeric names (e.g. "cpufreq", "cpu+5") are ignored.
func listCPUCores() ([]int, error) {
	entries, err := os.ReadDir(sysCPUDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", sysCPUDir, err)
	}

	var ids []int
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !strings.HasPrefix(name, "cpu") {
			continue
		}
		if id, ok := parseCoreSuffix(name[3:]); ok {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids, nil
}

// ListAllCPUCores counts the number of CPU cores exposed via sysfs.
func ListAllCPUCores() (int, error) {
	ids, err := listCPUCores()
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// coreOnline reports whether the given core is online. Cores without an
// "online" file (typically cpu0, and any core when hotplug is disabled)
// are always considered online.
func coreOnline(coreID int) (bool, error) {
	path := filepath.Join(sysCPUDir, fmt.Sprintf("cpu%d", coreID), "online")

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(data)) == "1", nil
}

// scanCoreStates returns the ascending core IDs together with their online
// states, read in a single pass.
func scanCoreStates() ([]int, []bool, error) {
	ids, err := listCPUCores()
	if err != nil {
		return nil, nil, err
	}

	states := make([]bool, len(ids))
	for k, id := range ids {
		state, err := coreOnline(id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read state of cpu%d: %w", id, err)
		}
		states[k] = state
	}
	return ids, states, nil
}

// GetCoreStates returns a slice of booleans indicating whether each core is
// online, aligned to the ascending core IDs. Read failures propagate as an
// error naming the offending core instead of being silently guessed, so all
// callers share one consistent view of the machine.
func GetCoreStates() ([]bool, error) {
	_, states, err := scanCoreStates()
	return states, err
}

// countOnline returns how many entries of states are true.
func countOnline(states []bool) int {
	n := 0
	for _, s := range states {
		if s {
			n++
		}
	}
	return n
}

// GetActiveCores counts how many CPU cores are currently online via sysfs.
func GetActiveCores() (int, error) {
	states, err := GetCoreStates()
	if err != nil {
		return 0, err
	}
	return countOnline(states), nil
}

// AllCPU returns the raw content of /proc/cpuinfo for debugging.
func AllCPU() (string, error) {
	data, err := os.ReadFile(procCPUInfoPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", procCPUInfoPath, err)
	}
	return string(data), nil
}

// GetInfo returns a fully populated CPUInfo struct built from a single
// sysfs scan, so total, active and per-core states always agree.
func GetInfo() (*CPUInfo, error) {
	model, err := GetCPUModel()
	if err != nil {
		return nil, err
	}

	ids, states, err := scanCoreStates()
	if err != nil {
		return nil, err
	}

	return &CPUInfo{
		Model:       model,
		TotalCores:  len(states),
		ActiveCores: countOnline(states),
		CoreIDs:     ids,
		CoreStates:  states,
	}, nil
}
