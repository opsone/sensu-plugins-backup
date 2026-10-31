package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"time"

	"github.com/jlaffaye/ftp"
	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-backup/internal/backup"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	User         string
	Password     string
	Host         string
	FTPDirectory string
	Directory    string
	Timeout      int
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-backup-ftp",
			Short:    "Check that local archives are present on an FTP server with the same size",
			Keyspace: "sensu.io/plugins/check-backup-ftp/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "ftp-user",
			Env:       "CHECK_BACKUP_FTP_USER",
			Argument:  "ftp-user",
			Shorthand: "u",
			Usage:     "FTP user (default: anonymous)",
			Value:     &plugin.User,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "ftp-password",
			Env:       "CHECK_BACKUP_FTP_PASSWORD",
			Argument:  "ftp-password",
			Shorthand: "p",
			Secret:    true,
			Usage:     "FTP password",
			Value:     &plugin.Password,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "ftp-host",
			Env:       "CHECK_BACKUP_FTP_HOST",
			Argument:  "ftp-host",
			Shorthand: "H", // -h is reserved for --help
			Usage:     "FTP host, with an optional port (default 21) (required)",
			Value:     &plugin.Host,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "ftp-directory",
			Env:       "CHECK_BACKUP_FTP_DIRECTORY",
			Argument:  "ftp-directory",
			Shorthand: "D",
			Usage:     "Remote directory holding the archives (default: login directory)",
			Value:     &plugin.FTPDirectory,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "directory-name",
			Env:       "CHECK_BACKUP_DIRECTORY",
			Argument:  "directory-name",
			Shorthand: "d",
			Default:   "/var/archives",
			Usage:     "Local directory holding the archives",
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
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if plugin.Host == "" {
		return sensu.CheckStateUnknown, fmt.Errorf("--ftp-host is required")
	}
	if plugin.Timeout <= 0 {
		return sensu.CheckStateUnknown, fmt.Errorf("--timeout must be greater than 0")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	files, err := backup.LocalFiles(plugin.Directory)
	if err != nil {
		return sensu.CheckStateUnknown, fmt.Errorf("reading %s: %w", plugin.Directory, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(plugin.Timeout)*time.Second)
	defer cancel()

	result := verify(ctx, files)
	backup.Print(plugin.Name, result)
	return result.State, nil
}

func verify(ctx context.Context, files []backup.File) backup.Result {
	remoteName := "FTP " + plugin.Host
	critical := func(err error) backup.Result {
		return backup.Result{State: sensu.CheckStateCritical, Message: fmt.Sprintf("%s: %v", remoteName, err)}
	}

	conn, err := ftp.Dial(hostWithPort(plugin.Host),
		ftp.DialWithContext(ctx),
		ftp.DialWithTimeout(time.Duration(plugin.Timeout)*time.Second),
		// Plain PASV using the address announced by the server, like the Ruby
		// version (use_pasv_ip = true).
		ftp.DialWithDisabledEPSV(true),
		ftp.DialWithTrustPasvIP(true),
	)
	if err != nil {
		return critical(err)
	}
	defer conn.Quit()

	// Abort pending commands when the timeout expires.
	stop := context.AfterFunc(ctx, func() { conn.Quit() })
	defer stop()

	user, password := plugin.User, plugin.Password
	if user == "" {
		user, password = "anonymous", "anonymous"
	}
	if err := conn.Login(user, password); err != nil {
		return critical(fmt.Errorf("login: %w", err))
	}
	if plugin.FTPDirectory != "" {
		if err := conn.ChangeDir(plugin.FTPDirectory); err != nil {
			return critical(fmt.Errorf("cd %s: %w", plugin.FTPDirectory, err))
		}
	}

	return backup.Verify(ctx, files, ftpRemote{conn}, remoteName)
}

type ftpRemote struct {
	conn *ftp.ServerConn
}

func (r ftpRemote) Size(ctx context.Context, name string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	size, err := r.conn.FileSize(name)
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) && protoErr.Code == ftp.StatusFileUnavailable {
		return 0, backup.ErrNotFound
	}
	return size, err
}

// hostWithPort appends the default FTP port when host has none.
func hostWithPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "21")
}
