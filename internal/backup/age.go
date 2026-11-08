package backup

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Set is a group of backups of the same thing, described by its newest one.
type Set struct {
	Name string    // e.g. "host-etc.*.tar.gz", "host:/etc,/var"
	Last string    // newest backup of the set, e.g. a file name or a snapshot ID
	Time time.Time // time of the newest backup
}

// datePart matches the date backup-manager puts in archive names
// (host-etc.20261108.tar.gz, host-etc.20261108.master.tar.gz).
var datePart = regexp.MustCompile(`\.\d{8}(\.master)?(\.|$)`)

// setName returns the name shared by every archive of a set, the date replaced by "*".
func setName(name string) string {
	if loc := datePart.FindStringSubmatchIndex(name); loc != nil {
		return name[:loc[0]] + ".*" + name[loc[4]:]
	}
	return name
}

// ArchiveSets groups archives by name without their backup-manager date and
// keeps the newest file (modification time) of each group.
func ArchiveSets(files []File) []Set {
	newest := map[string]File{}
	for _, f := range files {
		key := setName(f.Name)
		if cur, ok := newest[key]; !ok || f.ModTime.After(cur.ModTime) {
			newest[key] = f
		}
	}
	sets := make([]Set, 0, len(newest))
	for key, f := range newest {
		sets = append(sets, Set{Name: key, Last: f.Name, Time: f.ModTime})
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].Name < sets[j].Name })
	return sets
}

// CheckAge reports the sets whose newest backup is older than maxAge; no set at
// all is critical. kind names a set in messages (e.g. "archive set"), where
// names the place holding them (e.g. "/var/archives").
func CheckAge(sets []Set, kind, where string, maxAge time.Duration, now time.Time) Result {
	if len(sets) == 0 {
		return Result{sensu.CheckStateCritical, fmt.Sprintf("no %s in %s", kind, where)}
	}
	var stale []string
	for _, s := range sets {
		if age := now.Sub(s.Time); age > maxAge {
			stale = append(stale, fmt.Sprintf("%s (last %s, %s ago)", s.Name, s.Last, hours(age)))
		}
	}
	if len(stale) == 0 {
		return Result{sensu.CheckStateOK, fmt.Sprintf("%d %s(s) newer than %s in %s", len(sets), kind, hours(maxAge), where)}
	}
	sort.Strings(stale)
	return Result{sensu.CheckStateCritical, fmt.Sprintf("%d %s(s) older than %s in %s: %s", len(stale), kind, hours(maxAge), where, strings.Join(stale, ", "))}
}

func hours(d time.Duration) string {
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// Merge combines results into one: the most severe state, messages joined.
func Merge(results ...Result) Result {
	var merged Result
	var messages []string
	for _, r := range results {
		if severity(r.State) > severity(merged.State) {
			merged.State = r.State
		}
		messages = append(messages, r.Message)
	}
	merged.Message = strings.Join(messages, "; ")
	return merged
}

// severity orders states: critical outranks unknown.
func severity(state int) int {
	switch state {
	case sensu.CheckStateOK:
		return 0
	case sensu.CheckStateWarning:
		return 1
	case sensu.CheckStateCritical:
		return 3
	default:
		return 2
	}
}

// DefaultMaxAge suits daily backups, with some slack for the run duration.
const DefaultMaxAge = 26

// MaxAgeOption declares --max-age, shared by every check.
func MaxAgeOption(value *int) *sensu.PluginConfigOption[int] {
	return &sensu.PluginConfigOption[int]{
		Path:     "max-age",
		Env:      "CHECK_BACKUP_MAX_AGE",
		Argument: "max-age",
		Default:  DefaultMaxAge,
		Usage:    "Maximum age in hours of the newest backup of each set, 0 disables the age check",
		Value:    value,
	}
}

// ValidateMaxAge checks the --max-age value.
func ValidateMaxAge(maxAge int) error {
	if maxAge < 0 {
		return fmt.Errorf("--max-age must be 0 (disabled) or greater")
	}
	return nil
}

// WithArchiveAge adds the age check of the local archives to r when maxAge
// (hours) is set.
func WithArchiveAge(r Result, files []File, dir string, maxAge int) Result {
	if maxAge == 0 {
		return r
	}
	return Merge(r, CheckAge(ArchiveSets(files), "archive set", dir, time.Duration(maxAge)*time.Hour, time.Now()))
}
