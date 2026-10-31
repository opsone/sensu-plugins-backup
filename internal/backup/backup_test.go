package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

type fakeRemote struct {
	sizes map[string]int64
	err   error
}

func (r fakeRemote) Size(_ context.Context, name string) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	size, ok := r.sizes[name]
	if !ok {
		return 0, ErrNotFound
	}
	return size, nil
}

func TestLocalFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("db.sql.gz", "12345")
	write("db.sql.gz.md5", "abc")
	write("files.tar.gz", "")
	write(".lock", "x")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "db.sql.gz"), filepath.Join(dir, "latest.sql.gz")); err != nil {
		t.Fatal(err)
	}

	got, err := LocalFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []File{{"db.sql.gz", 5}, {"files.tar.gz", 0}, {"latest.sql.gz", 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocalFiles() = %v, want %v", got, want)
	}

	if _, err := LocalFiles(filepath.Join(dir, "missing")); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func TestVerify(t *testing.T) {
	files := []File{{"b.tar.gz", 10}, {"a.sql.gz", 20}, {"c.tar.gz", 30}}

	tests := []struct {
		name   string
		files  []File
		remote fakeRemote
		want   Result
	}{
		{
			name:   "no file",
			remote: fakeRemote{},
			want:   Result{sensu.CheckStateOK, "0 file(s) present in S3 bucket x"},
		},
		{
			name:   "all present",
			files:  files,
			remote: fakeRemote{sizes: map[string]int64{"a.sql.gz": 20, "b.tar.gz": 10, "c.tar.gz": 30}},
			want:   Result{sensu.CheckStateOK, "3 file(s) present in S3 bucket x"},
		},
		{
			name:   "missing and size mismatch, sorted",
			files:  files,
			remote: fakeRemote{sizes: map[string]int64{"c.tar.gz": 29}},
			want:   Result{sensu.CheckStateCritical, "3 file(s) in trouble in S3 bucket x: a.sql.gz (missing), b.tar.gz (missing), c.tar.gz (size 30 bytes, remote 29)"},
		},
		{
			name:   "remote error",
			files:  files,
			remote: fakeRemote{err: errors.New("access denied")},
			want:   Result{sensu.CheckStateCritical, "S3 bucket x: access denied"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Verify(context.Background(), tt.files, tt.remote, "S3 bucket x"); got != tt.want {
				t.Errorf("Verify() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
