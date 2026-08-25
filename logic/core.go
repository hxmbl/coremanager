package logic

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// EnableAll enables every offline secondary CPU core and returns how many
// cores were actually changed. The boot core (cpu0) is never touched.
func EnableAll(verbose bool) (int, error) {
	ids, states, err := scanCoreStates()
	if err != nil {
		return 0, err
	}

	changed := 0
	for k, id := range ids {
		if id == 0 || states[k] {
			continue
		}
		if verbose {
			fmt.Printf("  cpu%d -> enabling\n", id)
		}
		if err := setCoreOnline(id, true); err != nil {
			return changed, fmt.Errorf("enabled %d core(s) before failure; failed to enable cpu%d: %w", changed, id, err)
		}
		changed++
	}

	if verbose {
		fmt.Printf("Done. Enabled %d secondary core(s).\n", changed)
	}
	return changed, nil
}

// DisableAll disables every online secondary CPU core and returns how many
// cores were actually changed. The boot core (cpu0) is never touched.
func DisableAll(verbose bool) (int, error) {
	ids, states, err := scanCoreStates()
	if err != nil {
		return 0, err
	}

	changed := 0
	for k, id := range ids {
		if id == 0 || !states[k] {
			continue
		}
		if verbose {
			fmt.Printf("  cpu%d -> disabling\n", id)
		}
		if err := setCoreOnline(id, false); err != nil {
			return changed, fmt.Errorf("disabled %d core(s) before failure; failed to disable cpu%d: %w", changed, id, err)
		}
		changed++
	}

	if verbose {
		fmt.Printf("Done. Disabled %d secondary core(s).\n", changed)
	}
	return changed, nil
}

// Enable enables exactly `count` offline secondary cores, starting from the
// lowest-numbered offline core. It returns the number of cores enabled.
func Enable(count int, verbose bool) (int, error) {
	if count < 1 {
		return 0, errors.New("must enable at least 1 core")
	}

	ids, states, err := scanCoreStates()
	if err != nil {
		return 0, err
	}

	active := countOnline(states)
	disabled := len(states) - active

	if verbose {
		fmt.Printf("Requested: enable %d core(s)\n", count)
		fmt.Printf("Currently: %d active, %d disabled, %d total\n", active, disabled, len(states))
	}

	if count > disabled {
		return 0, fmt.Errorf("cannot enable %d core(s). Only %d disabled core(s) available", count, disabled)
	}

	enabled := 0
	for k, id := range ids {
		if id == 0 || states[k] {
			continue
		}
		if verbose {
			fmt.Printf("  cpu%d -> enabling\n", id)
		}
		if err := setCoreOnline(id, true); err != nil {
			return enabled, fmt.Errorf("enabled %d of %d core(s); failed to enable cpu%d: %w", enabled, count, id, err)
		}
		enabled++
		if enabled >= count {
			break
		}
	}

	if verbose {
		fmt.Printf("Done. Enabled %d core(s). Active cores: %d\n", enabled, active+enabled)
	}
	return enabled, nil
}

// Disable disables exactly `count` online secondary cores (never cpu0),
// starting from the lowest-numbered online core. It returns the number of
// cores disabled.
func Disable(count int, verbose bool) (int, error) {
	if count < 1 {
		return 0, errors.New("must disable at least 1 core")
	}

	ids, states, err := scanCoreStates()
	if err != nil {
		return 0, err
	}

	active := countOnline(states)
	canDisable := active - 1 // at least one core must stay online
	if canDisable < 0 {
		canDisable = 0
	}

	if verbose {
		fmt.Printf("Requested: disable %d core(s)\n", count)
		fmt.Printf("Currently: %d active, %d disabled, %d total\n", active, len(states)-active, len(states))
	}

	if count > canDisable {
		return 0, fmt.Errorf("cannot disable %d core(s). Only %d core(s) can be disabled (at least one core must stay online)", count, canDisable)
	}

	disabledCount := 0
	for k, id := range ids {
		if id == 0 || !states[k] {
			continue
		}
		if verbose {
			fmt.Printf("  cpu%d -> disabling\n", id)
		}
		if err := setCoreOnline(id, false); err != nil {
			return disabledCount, fmt.Errorf("disabled %d of %d core(s); failed to disable cpu%d: %w", disabledCount, count, id, err)
		}
		disabledCount++
		if disabledCount >= count {
			break
		}
	}

	if verbose {
		fmt.Printf("Done. Disabled %d core(s). Active cores: %d\n", disabledCount, active-disabledCount)
	}
	return disabledCount, nil
}

// setCoreOnline writes the desired state to the sysfs "online" file for the
// given core and verifies that the kernel actually applied it. Writing
// requires root privileges; permission failures produce an actionable
// message instead of shelling out through sudo.
func setCoreOnline(coreID int, online bool) error {
	value := "0"
	if online {
		value = "1"
	}
	path := filepath.Join(sysCPUDir, fmt.Sprintf("cpu%d", coreID), "online")

	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("%s: permission denied (re-run with sudo)", path)
		}
		return err
	}

	return verifyCoreState(coreID, online)
}

// verifyCoreState re-reads a core's state and errors if it does not match
// the desired value.
func verifyCoreState(coreID int, want bool) error {
	path := filepath.Join(sysCPUDir, fmt.Sprintf("cpu%d", coreID), "online")

	got, err := coreOnline(coreID)
	if err != nil {
		return fmt.Errorf("could not verify cpu%d state after write: %w", coreID, err)
	}
	if got == want {
		return nil
	}
	state := "offline"
	if got {
		state = "online"
	}
	return fmt.Errorf("write to %s did not take effect (kernel still reports core %s)", path, state)
}
