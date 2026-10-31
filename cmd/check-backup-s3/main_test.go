package main

import "testing"

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		host     string
		secure   bool
		wantErr  bool
	}{
		{endpoint: "https://s3.eu-west-3.amazonaws.com", host: "s3.eu-west-3.amazonaws.com", secure: true},
		{endpoint: "https://s3.fr-par.scw.cloud/", host: "s3.fr-par.scw.cloud", secure: true},
		{endpoint: "http://minio:9000", host: "minio:9000", secure: false},
		{endpoint: "s3.gra.io.cloud.ovh.net", host: "s3.gra.io.cloud.ovh.net", secure: true},
		{endpoint: "ftp://host", wantErr: true},
		{endpoint: "https://host/bucket", wantErr: true},
		{endpoint: "", wantErr: true},
	}

	for _, tt := range tests {
		host, secure, err := parseEndpoint(tt.endpoint)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			continue
		}
		if host != tt.host || secure != tt.secure {
			t.Errorf("parseEndpoint(%q) = %q, %v, want %q, %v", tt.endpoint, host, secure, tt.host, tt.secure)
		}
	}
}
