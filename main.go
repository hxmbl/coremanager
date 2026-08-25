package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hxmbl/coremanager/logic"

	"github.com/spf13/cobra"
)

const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorReset  = "\033[0m"
)

var (
	version  = "dev"
	verbose  bool
	colorOut bool
	colorErr bool
)

// isCharDevice reports whether f refers to a terminal-like device.
func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// detectColors enables ANSI output only for interactive streams, honoring
// the NO_COLOR convention and the "dumb" terminal type.
func detectColors() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return
	}
	colorOut = isCharDevice(os.Stdout)
	colorErr = isCharDevice(os.Stderr)
}

func paint(toStderr bool, code, s string) string {
	enabled := colorOut
	if toStderr {
		enabled = colorErr
	}
	if !enabled {
		return s
	}
	return code + s + colorReset
}

func green(s string) string  { return paint(false, colorGreen, s) }
func red(s string) string    { return paint(true, colorRed, s) }
func yellow(s string) string { return paint(false, colorYellow, s) }
func blue(s string) string   { return paint(false, colorBlue, s) }

func main() {
	detectColors()

	rootCmd := &cobra.Command{
		Use:           "coremanager",
		Short:         "A simple CLI tool to manage CPU cores on Linux",
		Long:          "CoreManager - Manage CPU cores dynamically to save battery or reduce heat.",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	disableCmd := &cobra.Command{
		Use:     "dc [N|all]",
		Aliases: []string{"disable-cores"},
		Short:   "Disable a number of CPU cores",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableCores(args[0])
		},
	}

	enableCmd := &cobra.Command{
		Use:     "ec [N|all]",
		Aliases: []string{"enable-cores"},
		Short:   "Enable a number of CPU cores",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnableCores(args[0])
		},
	}

	coreCountCmd := &cobra.Command{
		Use:     "cc",
		Aliases: []string{"core-count"},
		Short:   "Display the total and active CPU core counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCoreCount()
		},
	}

	cpuModelCmd := &cobra.Command{
		Use:     "cm",
		Aliases: []string{"cpu-model"},
		Short:   "Display the CPU model name",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCPUModel()
		},
	}

	debugCmd := &cobra.Command{
		Use:     "debug",
		Aliases: []string{"debug-info"},
		Short:   "Display detailed CPU information for debugging",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDebugInfo()
		},
	}

	rootCmd.AddCommand(disableCmd, enableCmd, coreCountCmd, cpuModelCmd, debugCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, red(fmt.Sprintf("Error: %v", err)))
		os.Exit(1)
	}
}

func runDisableCores(arg string) error {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return fmt.Errorf("specify how many cores to disable, or 'all'")
	}
	switch strings.ToLower(arg) {
	case "all", "a":
		if verbose {
			fmt.Println(yellow("Disabling all secondary cores..."))
		}
		n, err := logic.DisableAll(verbose)
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Println(yellow("No secondary cores needed disabling."))
		} else {
			fmt.Println(green(fmt.Sprintf("Disabled %d secondary core(s).", n)))
		}
		return nil
	}

	target, err := strconv.Atoi(arg)
	if err != nil {
		return fmt.Errorf("'%s' is not a valid number or 'all'", arg)
	}
	if target < 1 {
		return fmt.Errorf("must disable at least 1 core")
	}

	n, err := logic.Disable(target, verbose)
	if err != nil {
		return err
	}
	fmt.Println(green(fmt.Sprintf("Disabled %d core(s).", n)))
	return nil
}

func runEnableCores(arg string) error {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return fmt.Errorf("specify how many cores to enable, or 'all'")
	}
	switch strings.ToLower(arg) {
	case "all", "a":
		if verbose {
			fmt.Println(yellow("Enabling all secondary cores..."))
		}
		n, err := logic.EnableAll(verbose)
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Println(yellow("No secondary cores needed enabling."))
		} else {
			fmt.Println(green(fmt.Sprintf("Enabled %d secondary core(s).", n)))
		}
		return nil
	}

	target, err := strconv.Atoi(arg)
	if err != nil {
		return fmt.Errorf("'%s' is not a valid number or 'all'", arg)
	}
	if target < 1 {
		return fmt.Errorf("must enable at least 1 core")
	}

	n, err := logic.Enable(target, verbose)
	if err != nil {
		return err
	}
	fmt.Println(green(fmt.Sprintf("Enabled %d core(s).", n)))
	return nil
}

func runCoreCount() error {
	info, err := logic.GetInfo()
	if err != nil {
		return err
	}
	fmt.Println(blue(fmt.Sprintf("Total CPU cores: %d", info.TotalCores)))
	fmt.Println(blue(fmt.Sprintf("Active CPU cores: %d", info.ActiveCores)))
	return nil
}

func runCPUModel() error {
	model, err := logic.GetCPUModel()
	if err != nil {
		return err
	}
	fmt.Println(blue(fmt.Sprintf("CPU Model: %s", model)))
	return nil
}

func runDebugInfo() error {
	info, err := logic.GetInfo()
	if err != nil {
		return err
	}

	fmt.Println(blue(fmt.Sprintf("CPU Model: %s", info.Model)))
	fmt.Println(blue(fmt.Sprintf("Total CPU cores: %d", info.TotalCores)))
	fmt.Println(blue(fmt.Sprintf("Active CPU cores: %d", info.ActiveCores)))
	fmt.Println(blue(fmt.Sprintf("Core IDs: %v", info.CoreIDs)))
	fmt.Println(blue(fmt.Sprintf("Core states (0=offline, 1=online): %v", info.CoreStates)))

	fmt.Print("Type 'a' for a lot of info. Press Enter to continue... ")
	input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(input), "a") {
		fmt.Println(blue("Detailed info:"))
		raw, err := logic.AllCPU()
		if err != nil {
			return err
		}
		fmt.Println(raw)
	}
	return nil
}
