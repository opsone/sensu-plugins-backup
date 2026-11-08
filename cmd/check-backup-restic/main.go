package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

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
	MaxAge         int
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-backup-restic",
			Short:    "Check the integrity of a restic repository with restic check, and the age of its snapshots",
			Keyspace: "sensu.io/plugins/check-backup-restic/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "env-file",
			Env:       "CHECK_BACKUP_RESTIC_ENV_FILE",
			Argument:  "env-file",
			Shorthand: "e",
			Usage:     "Environment file defining the restic repository and credentials (required)",
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
		backup.MaxAgeOption(&plugin.MaxAge),
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if plugin.EnvFile == "" {
		return sensu.CheckStateUnknown, fmt.Errorf("--env-file is required")
	}
	if _, err := resticArgs(plugin.ReadData, plugin.ReadDataSubset); err != nil {
		return sensu.CheckStateUnknown, err
	}
	if err := backup.ValidateMaxAge(plugin.MaxAge); err != nil {
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

	// Later entries win: the env file overrides the agent environment.
	environ := os.Environ()
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}

	args, _ := resticArgs(plugin.ReadData, plugin.ReadDataSubset)
	cmd := exec.Command(plugin.ResticBinary, args...)
	cmd.Env = environ
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return sensu.CheckStateUnknown, fmt.Errorf("running %s: %w", plugin.ResticBinary, err)
	}

	result := evaluate(mode(plugin.ReadData, plugin.ReadDataSubset), out.String(), err == nil)
	// A failing repository makes the snapshot age meaningless.
	if result.State == sensu.CheckStateOK && plugin.MaxAge > 0 {
		result = backup.Merge(result, snapshotAge(environ, time.Duration(plugin.MaxAge)*time.Hour))
	}
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

// snapshotAge checks that the newest snapshot of every host and paths group is
// younger than maxAge.
func snapshotAge(environ []string, maxAge time.Duration) backup.Result {
	cmd := exec.Command(plugin.ResticBinary, "snapshots", "--latest", "1", "--group-by", "host,paths", "--json")
	cmd.Env = environ
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return backup.Result{State: sensu.CheckStateCritical, Message: "restic snapshots failed: " + msg}
	}
	sets, err := snapshotSets(stdout.Bytes())
	if err != nil {
		return backup.Result{State: sensu.CheckStateCritical, Message: fmt.Sprintf("parsing restic snapshots: %v", err)}
	}
	return backup.CheckAge(sets, "snapshot group", "repository", maxAge, time.Now())
}

// snapshotSets parses `restic snapshots --group-by host,paths --json`, keeping
// the newest snapshot of each group.
func snapshotSets(data []byte) ([]backup.Set, error) {
	var groups []struct {
		GroupKey struct {
			Hostname string   `json:"hostname"`
			Paths    []string `json:"paths"`
		} `json:"group_key"`
		Snapshots []struct {
			Time    time.Time `json:"time"`
			ShortID string    `json:"short_id"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	var sets []backup.Set
	for _, g := range groups {
		if len(g.Snapshots) == 0 {
			continue
		}
		set := backup.Set{Name: g.GroupKey.Hostname + ":" + strings.Join(g.GroupKey.Paths, ",")}
		for _, snap := range g.Snapshots {
			if set.Last == "" || snap.Time.After(set.Time) {
				set.Last, set.Time = "snapshot "+snap.ShortID, snap.Time
			}
		}
		sets = append(sets, set)
	}
	return sets, nil
}
