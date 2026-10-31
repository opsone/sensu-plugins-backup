package main

import "testing"

func TestHostWithPort(t *testing.T) {
	tests := map[string]string{
		"ftp.example.com":      "ftp.example.com:21",
		"ftp.example.com:2121": "ftp.example.com:2121",
		"192.0.2.10":           "192.0.2.10:21",
		"2001:db8::1":          "[2001:db8::1]:21",
		"[2001:db8::1]:2121":   "[2001:db8::1]:2121",
	}
	for host, want := range tests {
		if got := hostWithPort(host); got != want {
			t.Errorf("hostWithPort(%q) = %q, want %q", host, got, want)
		}
	}
}
