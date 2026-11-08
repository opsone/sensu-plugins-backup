package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-backup/internal/backup"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	AccessKey string
	SecretKey string
	Region    string
	Endpoint  string
	Bucket    string
	Directory string
	Timeout   int
	MaxAge    int
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-backup-s3",
			Short:    "Check that local archives are present in an S3 bucket with the same size, and recent",
			Keyspace: "sensu.io/plugins/check-backup-s3/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "s3-access-key",
			Env:       "CHECK_BACKUP_S3_ACCESS_KEY",
			Argument:  "s3-access-key",
			Shorthand: "a",
			Usage:     "S3 access key (fallback: AWS_ACCESS_KEY_ID, AWS_ACCESS_KEY, ~/.aws/credentials)",
			Value:     &plugin.AccessKey,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "s3-secret-key",
			Env:       "CHECK_BACKUP_S3_SECRET_KEY",
			Argument:  "s3-secret-key",
			Shorthand: "k",
			Secret:    true,
			Usage:     "S3 secret key (fallback: AWS_SECRET_ACCESS_KEY, AWS_SECRET_KEY, ~/.aws/credentials)",
			Value:     &plugin.SecretKey,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "s3-region",
			Env:       "CHECK_BACKUP_S3_REGION",
			Argument:  "s3-region",
			Shorthand: "r",
			Usage:     "S3 region, e.g. fr-par (Scaleway), gra (OVH) (required)",
			Value:     &plugin.Region,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "s3-endpoint",
			Env:       "CHECK_BACKUP_S3_ENDPOINT",
			Argument:  "s3-endpoint",
			Shorthand: "e",
			Usage:     "S3 endpoint URL, e.g. https://s3.fr-par.scw.cloud, https://s3.gra.io.cloud.ovh.net (required)",
			Value:     &plugin.Endpoint,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "bucket-name",
			Env:       "CHECK_BACKUP_S3_BUCKET",
			Argument:  "bucket-name",
			Shorthand: "b",
			Usage:     "S3 bucket holding the archives (required)",
			Value:     &plugin.Bucket,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "directory-name",
			Env:       "CHECK_BACKUP_DIRECTORY",
			Argument:  "directory-name",
			Shorthand: "d",
			Usage:     "Local directory holding the archives, e.g. /var/archives (required)",
			Value:     &plugin.Directory,
		},
		&sensu.PluginConfigOption[int]{
			Path:     "timeout",
			Env:      "CHECK_BACKUP_TIMEOUT",
			Argument: "timeout",
			Default:  60,
			Usage:    "Timeout of the whole verification in seconds",
			Value:    &plugin.Timeout,
		},
		backup.MaxAgeOption(&plugin.MaxAge),
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	required := []struct{ flag, value string }{
		{"--bucket-name", plugin.Bucket},
		{"--s3-endpoint", plugin.Endpoint},
		{"--s3-region", plugin.Region},
		{"--directory-name", plugin.Directory},
	}
	for _, r := range required {
		if r.value == "" {
			return sensu.CheckStateUnknown, fmt.Errorf("%s is required", r.flag)
		}
	}
	if plugin.Timeout <= 0 {
		return sensu.CheckStateUnknown, fmt.Errorf("--timeout must be greater than 0")
	}
	if err := backup.ValidateMaxAge(plugin.MaxAge); err != nil {
		return sensu.CheckStateUnknown, err
	}
	if _, _, err := parseEndpoint(plugin.Endpoint); err != nil {
		return sensu.CheckStateUnknown, err
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	files, err := backup.LocalFiles(plugin.Directory)
	if err != nil {
		return sensu.CheckStateUnknown, fmt.Errorf("reading %s: %w", plugin.Directory, err)
	}

	host, secure, _ := parseEndpoint(plugin.Endpoint)
	client, err := minio.New(host, &minio.Options{
		Creds:  credentialsFor(plugin.AccessKey, plugin.SecretKey),
		Secure: secure,
		Region: plugin.Region,
	})
	if err != nil {
		return sensu.CheckStateUnknown, fmt.Errorf("creating S3 client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(plugin.Timeout)*time.Second)
	defer cancel()

	result := backup.Verify(ctx, files, s3Remote{client, plugin.Bucket}, "S3 bucket "+plugin.Bucket)
	result = backup.WithArchiveAge(result, files, plugin.Directory, plugin.MaxAge)
	backup.Print(plugin.Name, result)
	return result.State, nil
}

type s3Remote struct {
	client *minio.Client
	bucket string
}

func (r s3Remote) Size(ctx context.Context, name string) (int64, error) {
	info, err := r.client.StatObject(ctx, r.bucket, name, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return 0, backup.ErrNotFound
		}
		return 0, err
	}
	return info.Size, nil
}

// parseEndpoint splits an endpoint URL ("https://s3.fr-par.scw.cloud") into the
// host expected by minio and whether TLS is used. A bare host means HTTPS.
func parseEndpoint(endpoint string) (host string, secure bool, err error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		u, err = url.Parse("https://" + endpoint)
	}
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") {
		return "", false, fmt.Errorf("invalid --s3-endpoint %q", endpoint)
	}
	switch u.Scheme {
	case "https":
		return u.Host, true, nil
	case "http":
		return u.Host, false, nil
	default:
		return "", false, fmt.Errorf("invalid --s3-endpoint %q: scheme must be http or https", endpoint)
	}
}

// credentialsFor uses the given keys, or falls back to AWS_ACCESS_KEY_ID /
// AWS_SECRET_ACCESS_KEY (or AWS_ACCESS_KEY / AWS_SECRET_KEY) then ~/.aws/credentials. No instance role lookup: the
// AWS metadata endpoint is useless on Scaleway or OVH.
func credentialsFor(accessKey, secretKey string) *credentials.Credentials {
	if accessKey != "" || secretKey != "" {
		return credentials.NewStaticV4(accessKey, secretKey, "")
	}
	return credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.FileAWSCredentials{},
	})
}
