package backup

import (
	"reflect"
	"testing"
	"time"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func ago(h int) time.Time { return now.Add(-time.Duration(h) * time.Hour) }

func TestSetName(t *testing.T) {
	tests := map[string]string{
		"host-etc.20261108.tar.gz":        "host-etc.*.tar.gz",
		"host-etc.20261108.master.tar.gz": "host-etc.*.tar.gz",
		"host-mysql-db.20261108.sql.gz":   "host-mysql-db.*.sql.gz",
		"host-etc.20261108":               "host-etc.*",
		"dump.sql.gz":                     "dump.sql.gz",
		"host-etc.2026110.tar.gz":         "host-etc.2026110.tar.gz",
	}
	for name, want := range tests {
		if got := setName(name); got != want {
			t.Errorf("setName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestArchiveSets(t *testing.T) {
	files := []File{
		{Name: "h-mysql-db.20261108.sql.gz", ModTime: ago(6)},
		{Name: "h-etc.20261107.master.tar.gz", ModTime: ago(30)},
		{Name: "h-etc.20261108.tar.gz", ModTime: ago(6)},
		{Name: "h-mysql-db.20261107.sql.gz", ModTime: ago(30)},
	}
	want := []Set{
		{"h-etc.*.tar.gz", "h-etc.20261108.tar.gz", ago(6)},
		{"h-mysql-db.*.sql.gz", "h-mysql-db.20261108.sql.gz", ago(6)},
	}
	if got := ArchiveSets(files); !reflect.DeepEqual(got, want) {
		t.Errorf("ArchiveSets() = %v, want %v", got, want)
	}
}

func TestCheckAge(t *testing.T) {
	tests := []struct {
		name string
		sets []Set
		want Result
	}{
		{
			name: "no set",
			want: Result{sensu.CheckStateCritical, "no archive set in /var/archives"},
		},
		{
			name: "every set up to date",
			sets: []Set{{"a", "a1", ago(6)}, {"b", "b1", ago(25)}},
			want: Result{sensu.CheckStateOK, "2 archive set(s) newer than 26h in /var/archives"},
		},
		{
			name: "stale sets, sorted",
			sets: []Set{{"c", "c1", ago(6)}, {"b", "b1", ago(54)}, {"a", "a1", ago(30)}},
			want: Result{sensu.CheckStateCritical, "2 archive set(s) older than 26h in /var/archives: a (last a1, 30h ago), b (last b1, 54h ago)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckAge(tt.sets, "archive set", "/var/archives", 26*time.Hour, now); got != tt.want {
				t.Errorf("CheckAge() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	ok := Result{sensu.CheckStateOK, "ok"}
	crit := Result{sensu.CheckStateCritical, "crit"}
	unknown := Result{sensu.CheckStateUnknown, "unknown"}
	if got, want := Merge(ok, ok), (Result{sensu.CheckStateOK, "ok; ok"}); got != want {
		t.Errorf("Merge() = %+v, want %+v", got, want)
	}
	if got, want := Merge(ok, crit), (Result{sensu.CheckStateCritical, "ok; crit"}); got != want {
		t.Errorf("Merge() = %+v, want %+v", got, want)
	}
	if got, want := Merge(crit, unknown), (Result{sensu.CheckStateCritical, "crit; unknown"}); got != want {
		t.Errorf("Merge() = %+v, want %+v", got, want)
	}
}
