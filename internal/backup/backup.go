// Package backup holds the logic shared by the checks comparing local archives
// with their remote copy (S3, FTP).
package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// ErrNotFound is returned by a Remote when the file does not exist on it.
var ErrNotFound = errors.New("not found")

// File is a local archive to look for on the remote.
type File struct {
	Name string
	Size int64
}

// Remote returns the size of a file stored remotely, or ErrNotFound.
type Remote interface {
	Size(ctx context.Context, name string) (int64, error)
}

// Result is the outcome of a check.
type Result struct {
	State   int
	Message string
}

// LocalFiles lists the files of dir, skipping subdirectories, hidden and .md5 files.
func LocalFiles(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, e := range entries {
		// Hidden files are skipped like Dir.glob("*") did in the Ruby version.
		if strings.HasPrefix(e.Name(), ".") || filepath.Ext(e.Name()) == ".md5" {
			continue
		}
		// Stat follows symlinks, like File.directory? / File.size did.
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		files = append(files, File{e.Name(), info.Size()})
	}
	return files, nil
}

// Verify checks that every file exists on the remote with the same size.
// remoteName describes the remote in messages (e.g. "S3 bucket backups").
// An error other than ErrNotFound stops the verification and is reported as critical.
func Verify(ctx context.Context, files []File, remote Remote, remoteName string) Result {
	var problems []string
	for _, f := range files {
		size, err := remote.Size(ctx, f.Name)
		switch {
		case errors.Is(err, ErrNotFound):
			problems = append(problems, fmt.Sprintf("%s (missing)", f.Name))
		case err != nil:
			return Result{sensu.CheckStateCritical, fmt.Sprintf("%s: %v", remoteName, err)}
		case size != f.Size:
			problems = append(problems, fmt.Sprintf("%s (size %d bytes, remote %d)", f.Name, f.Size, size))
		}
	}

	if len(problems) == 0 {
		return Result{sensu.CheckStateOK, fmt.Sprintf("%d file(s) present in %s", len(files), remoteName)}
	}
	sort.Strings(problems)
	return Result{sensu.CheckStateCritical, fmt.Sprintf("%d file(s) in trouble in %s: %s", len(problems), remoteName, strings.Join(problems, ", "))}
}

// Print writes the check output line, e.g. "check-backup-s3 OK: ...".
func Print(name string, r Result) {
	fmt.Printf("%s %s: %s\n", name, StateLabel(r.State), r.Message)
}

// StateLabel returns the human-readable label of a check state.
func StateLabel(state int) string {
	switch state {
	case sensu.CheckStateOK:
		return "OK"
	case sensu.CheckStateWarning:
		return "WARNING"
	case sensu.CheckStateCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}
