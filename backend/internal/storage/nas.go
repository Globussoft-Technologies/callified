package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// NASClient stores recordings on a NAS through its SFTP-only account.
type NASClient struct {
	host          string
	port          int
	user          string
	password      string
	basePath      string
	hostKeySHA256 string
}

func NewNASClient(host string, port int, user, password, basePath, hostKeySHA256 string) *NASClient {
	return &NASClient{
		host: host, port: port, user: user, password: password,
		basePath:      path.Clean("/" + strings.TrimSpace(basePath)),
		hostKeySHA256: strings.TrimSpace(hostKeySHA256),
	}
}

func (c *NASClient) hostKeyCallback() ssh.HostKeyCallback {
	if c.hostKeySHA256 == "" {
		// Some existing NAS installations do not expose a managed known_hosts
		// file. Operators should set NAS_HOST_KEY_SHA256 to pin the server key.
		return ssh.InsecureIgnoreHostKey() //nolint:gosec
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		for _, trusted := range strings.Split(c.hostKeySHA256, ",") {
			if strings.TrimSpace(trusted) == got {
				return nil
			}
		}
		return fmt.Errorf("NAS host key mismatch: got %s", got)
	}
}

func (c *NASClient) connect(ctx context.Context) (*ssh.Client, *sftp.Client, error) {
	addr := net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("dial NAS: %w", err)
	}
	sshConfig := &ssh.ClientConfig{
		User:            c.user,
		Auth:            []ssh.AuthMethod{ssh.Password(c.password)},
		HostKeyCallback: c.hostKeyCallback(),
		Timeout:         10 * time.Second,
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("authenticate NAS: %w", err)
	}
	sshClient := ssh.NewClient(clientConn, chans, reqs)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("start NAS SFTP: %w", err)
	}
	return sshClient, sftpClient, nil
}

func (c *NASClient) remotePath(key string) (string, error) {
	for _, segment := range strings.Split(strings.ReplaceAll(key, "\\", "/"), "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid NAS object key %q", key)
		}
	}
	clean := path.Clean("/" + key)
	if clean == "/" {
		return "", fmt.Errorf("invalid NAS object key %q", key)
	}
	return path.Join(c.basePath, strings.TrimPrefix(clean, "/")), nil
}

// UploadPublic uploads a recording and returns its authenticated backend URL.
func (c *NASClient) UploadPublic(ctx context.Context, key string, data []byte) (string, error) {
	remotePath, err := c.remotePath(key)
	if err != nil {
		return "", err
	}
	sshClient, sftpClient, err := c.connect(ctx)
	if err != nil {
		return "", err
	}
	defer sshClient.Close()
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(path.Dir(remotePath)); err != nil {
		return "", fmt.Errorf("create NAS directory: %w", err)
	}
	file, err := sftpClient.Create(remotePath)
	if err != nil {
		return "", fmt.Errorf("create NAS recording: %w", err)
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write NAS recording: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close NAS recording: %w", err)
	}

	rel := strings.TrimPrefix(path.Clean("/"+key), "/recordings/")
	return "/api/recordings/" + rel, nil
}

// NASFile keeps the SSH/SFTP connections alive while a remote recording is
// streamed by http.ServeContent.
type NASFile struct {
	*sftp.File
	sftpClient *sftp.Client
	sshClient  *ssh.Client
}

func (f *NASFile) Close() error {
	err := f.File.Close()
	_ = f.sftpClient.Close()
	_ = f.sshClient.Close()
	return err
}

// Open opens an existing NAS object for authenticated streaming.
func (c *NASClient) Open(ctx context.Context, key string) (*NASFile, os.FileInfo, error) {
	remotePath, err := c.remotePath(key)
	if err != nil {
		return nil, nil, err
	}
	sshClient, sftpClient, err := c.connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	file, err := sftpClient.Open(remotePath)
	if err != nil {
		_ = sftpClient.Close()
		_ = sshClient.Close()
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = sftpClient.Close()
		_ = sshClient.Close()
		return nil, nil, err
	}
	return &NASFile{File: file, sftpClient: sftpClient, sshClient: sshClient}, info, nil
}
