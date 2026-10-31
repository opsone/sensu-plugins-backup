package main

import (
	"reflect"
	"testing"

	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-backup/internal/backup"
)

func TestResticArgs(t *testing.T) {
	tests := []struct {
		readData bool
		subset   string
		want     []string
		wantErr  bool
	}{
		{want: []string{"check"}},
		{readData: true, want: []string{"check", "--read-data"}},
		{subset: "1/7", want: []string{"check", "--read-data-subset=1/7"}},
		{subset: "2.5%", want: []string{"check", "--read-data-subset=2.5%"}},
		{subset: "10", wantErr: true},
		{subset: "1/7; rm -rf /", wantErr: true},
		{readData: true, subset: "1/7", wantErr: true},
	}
	for _, tt := range tests {
		got, err := resticArgs(tt.readData, tt.subset)
		if (err != nil) != tt.wantErr || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("resticArgs(%v, %q) = %v, %v, want %v, wantErr %v", tt.readData, tt.subset, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestMode(t *testing.T) {
	if got := mode(false, ""); got != "QUICK" {
		t.Errorf("mode() = %q", got)
	}
	if got := mode(true, ""); got != "FULL" {
		t.Errorf("mode() = %q", got)
	}
	if got := mode(false, "10%"); got != "SUBSET 10%" {
		t.Errorf("mode() = %q", got)
	}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		success bool
		want    backup.Result
	}{
		{
			name:    "success keeps the last line",
			output:  "using temporary cache\ncheck snapshots, trees and blobs\nno errors were found\n",
			success: true,
			want:    backup.Result{State: sensu.CheckStateOK, Message: "[QUICK] no errors were found"},
		},
		{
			name:    "success without output",
			success: true,
			want:    backup.Result{State: sensu.CheckStateOK, Message: "[QUICK] restic check passed"},
		},
		{
			name:   "failure keeps the whole output",
			output: "Fatal: unable to open repository\nwrong password\n",
			want:   backup.Result{State: sensu.CheckStateCritical, Message: "[QUICK] restic check failed: Fatal: unable to open repository\nwrong password"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluate("QUICK", tt.output, tt.success); got != tt.want {
				t.Errorf("evaluate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
