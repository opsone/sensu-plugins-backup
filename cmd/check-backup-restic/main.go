package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-backup/internal/backup"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	EnvFile        string
	ResticBinary   string
	ReadData       bool
	ReadDataSubset string
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-backup-restic",
			Short:    "Check the integrity of a restic repository with restic check",
			Keyspace: "sensu.io/plugins/check-backup-restic/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "env-file",
			Env:       "CHECK_BACKUP_RESTIC_ENV_FILE",
			Argument:  "env-file",
			Shorthand: "e",
			Default:   "/etc/restic/env",
			Usage:     "Environment file defining the restic repository and credentials",
			Value:     &plugin.EnvFile,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "restic-binary",
			Env:       "CHECK_BACKUP_RESTIC_BINARY",
			Argument:  "restic-binary",
			Shorthand: "b",
			Default:   "restic",
			Usage:     "Path to the restic binary",
			Value:     &plugin.ResticBinary,
		},
		&sensu.PluginConfigOption[bool]{
			Path:     "read-data",
			Env:      "CHECK_BACKUP_RESTIC_READ_DATA",
			Argument: "read-data",
			Usage:    "Read all data blobs (thorough but slow, use at low frequency)",
			Value:    &plugin.ReadData,
		},
		&sensu.PluginConfigOption[string]{
			Path:     "read-data-subset",
			Env:      "CHECK_BACKUP_RESTIC_READ_DATA_SUBSET",
			Argument: "read-data-subset",
			Usage:    `Read a subset of data blobs: "n/t" (e.g. "1/7") or "x%" (e.g. "10%")`,
			Value:    &plugin.ReadDataSubset,
		},
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if _, err := resticArgs(plugin.ReadData, plugin.ReadDataSubset); err != nil {
		return sensu.CheckStateUnknown, err
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	env, err := godotenv.Read(plugin.EnvFile)
	if err != nil {
		// A missing or unreadable env file means the backups cannot run either.
		result := backup.Result{State: sensu.CheckStateCritical, Message: fmt.Sprintf("reading environment file: %v", err)}
		backup.Print(plugin.Name, result)
		return result.State, nil
	}

	args, _ := resticArgs(plugin.ReadData, plugin.ReadDataSubset)
	cmd := exec.Command(plugin.ResticBinary, args...)
	// Later entries win: the env file overrides the agent environment.
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return sensu.CheckStateUnknown, fmt.Errorf("running %s: %w", plugin.ResticBinary, err)
	}

	result := evaluate(mode(plugin.ReadData, plugin.ReadDataSubset), out.String(), err == nil)
	backup.Print(plugin.Name, result)
	return result.State, nil
}

var subsetPattern = regexp.MustCompile(`^(\d+/\d+|\d+(\.\d+)?%)$`)

// resticArgs builds the restic command line arguments.
func resticArgs(readData bool, subset string) ([]string, error) {
	switch {
	case readData && subset != "":
		return nil, errors.New("--read-data and --read-data-subset are mutually exclusive")
	case readData:
		return []string{"check", "--read-data"}, nil
	case subset != "":
		if !subsetPattern.MatchString(subset) {
			return nil, fmt.Errorf("invalid --read-data-subset %q (expected n/t or x%%)", subset)
		}
		return []string{"check", "--read-data-subset=" + subset}, nil
	default:
		return []string{"check"}, nil
	}
}

func mode(readData bool, subset string) string {
	switch {
	case readData:
		return "FULL"
	case subset != "":
		return "SUBSET " + subset
	default:
		return "QUICK"
	}
}

// evaluate turns the restic check output into a check result: the last line
// on success, the whole output on failure.
func evaluate(mode, output string, success bool) backup.Result {
	output = strings.TrimSpace(output)
	if !success {
		return backup.Result{State: sensu.CheckStateCritical, Message: fmt.Sprintf("[%s] restic check failed: %s", mode, output)}
	}
	last := "restic check passed"
	if output != "" {
		last = output[strings.LastIndex(output, "\n")+1:]
	}
	return backup.Result{State: sensu.CheckStateOK, Message: fmt.Sprintf("[%s] %s", mode, last)}
}
