package rssh

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const (
	MaxFileTransferBytes int64 = 500 * 1024 * 1024

	filePreviewLines    = 50
	filePreviewMaxBytes = 256 * 1024
)

type FileList struct {
	Path  string      `json:"path"`
	Items []FileEntry `json:"items"`
}

type FileEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type FileDownload struct {
	io.ReadCloser
	Path        string
	Name        string
	Size        int64
	Mode        string
	ModifiedAt  time.Time
	closeClient func() error
}

type FileUploadResult struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type FilePreview struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Lines     int    `json:"lines"`
	Truncated bool   `json:"truncated"`
}

func (s *Service) ListFilesOnConnection(connectionID, remotePath string) (FileList, error) {
	client, closeClient, err := s.openSFTPClient(connectionID)
	if err != nil {
		return FileList{}, err
	}
	defer closeClient()

	cleanPath := CleanRemotePath(remotePath)
	entries, err := client.ReadDir(cleanPath)
	if err != nil {
		return FileList{}, err
	}

	items := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		items = append(items, fileEntryFromInfo(cleanPath, entry))
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Type == "directory" && items[j].Type != "directory" {
			return true
		}
		if items[i].Type != "directory" && items[j].Type == "directory" {
			return false
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return FileList{
		Path:  cleanPath,
		Items: items,
	}, nil
}

func (s *Service) PreviewFileOnConnection(connectionID, remotePath string) (FilePreview, error) {
	download, err := s.OpenFileOnConnection(connectionID, remotePath)
	if err != nil {
		return FilePreview{}, err
	}
	defer download.Close()

	content, lines, truncated, err := readFilePreview(download)
	if err != nil {
		return FilePreview{}, err
	}

	return FilePreview{
		Path:      download.Path,
		Name:      download.Name,
		Content:   content,
		Lines:     lines,
		Truncated: truncated,
	}, nil
}

func (s *Service) OpenFileOnConnection(connectionID, remotePath string) (*FileDownload, error) {
	client, closeClient, err := s.openSFTPClient(connectionID)
	if err != nil {
		return nil, err
	}

	cleanPath := CleanRemotePath(remotePath)
	info, err := client.Stat(cleanPath)
	if err != nil {
		closeClient()
		return nil, err
	}
	if info.IsDir() {
		closeClient()
		return nil, fmt.Errorf("remote path is a directory")
	}

	file, err := client.Open(cleanPath)
	if err != nil {
		closeClient()
		return nil, err
	}

	return &FileDownload{
		ReadCloser:  file,
		Path:        cleanPath,
		Name:        path.Base(cleanPath),
		Size:        info.Size(),
		Mode:        info.Mode().String(),
		ModifiedAt:  info.ModTime(),
		closeClient: closeClient,
	}, nil
}

func (s *Service) UploadFileOnConnection(connectionID, directory, filename string, reader io.Reader) (FileUploadResult, error) {
	name, err := cleanUploadFilename(filename)
	if err != nil {
		return FileUploadResult{}, err
	}

	client, closeClient, err := s.openSFTPClient(connectionID)
	if err != nil {
		return FileUploadResult{}, err
	}
	defer closeClient()

	directory = CleanRemotePath(directory)
	remotePath := JoinRemotePath(directory, name)
	file, err := client.OpenFile(remotePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return FileUploadResult{}, err
	}

	written, copyErr := io.Copy(file, maxBytesReader(reader, MaxFileTransferBytes))
	closeErr := file.Close()
	if copyErr != nil {
		_ = client.Remove(remotePath)
		return FileUploadResult{}, copyErr
	}
	if closeErr != nil {
		_ = client.Remove(remotePath)
		return FileUploadResult{}, closeErr
	}

	return FileUploadResult{
		Path: remotePath,
		Name: name,
		Size: written,
	}, nil
}

type cappedReader struct {
	reader    io.Reader
	remaining int64
}

func maxBytesReader(reader io.Reader, max int64) io.Reader {
	return &cappedReader{
		reader:    reader,
		remaining: max,
	}
}

func (r *cappedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("file exceeds maximum transfer size of %d MiB", MaxFileTransferBytes/(1024*1024))
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (s *Service) openSFTPClient(connectionID string) (*sftp.Client, func() error, error) {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return nil, nil, fmt.Errorf("connection id is required")
	}

	client, ok := users.GetClientConnection(connectionID)
	if !ok || client == nil {
		return nil, nil, fmt.Errorf("host is not currently connected")
	}

	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		return nil, nil, err
	}
	go ssh.DiscardRequests(requests)

	okReply, err := channel.SendRequest("subsystem", true, ssh.Marshal(struct {
		Name string
	}{Name: "sftp"}))
	if err != nil {
		channel.Close()
		return nil, nil, err
	}
	if !okReply {
		channel.Close()
		return nil, nil, fmt.Errorf("client refused sftp subsystem")
	}

	sftpClient, err := sftp.NewClientPipe(channel, channel)
	if err != nil {
		channel.Close()
		return nil, nil, err
	}

	return sftpClient, sftpClient.Close, nil
}

func (d *FileDownload) Close() error {
	var closeErr error
	if d.ReadCloser != nil {
		closeErr = d.ReadCloser.Close()
	}
	if d.closeClient != nil {
		if err := d.closeClient(); closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func fileEntryFromInfo(parent string, info os.FileInfo) FileEntry {
	entryType := "other"
	mode := info.Mode()
	switch {
	case mode.IsDir():
		entryType = "directory"
	case mode.IsRegular():
		entryType = "file"
	case mode&os.ModeSymlink != 0:
		entryType = "symlink"
	}

	return FileEntry{
		Name:       info.Name(),
		Path:       JoinRemotePath(parent, info.Name()),
		Type:       entryType,
		Size:       info.Size(),
		Mode:       mode.String(),
		ModifiedAt: info.ModTime(),
	}
}

func CleanRemotePath(remotePath string) string {
	value := strings.TrimSpace(remotePath)
	if value == "" {
		return "/"
	}
	value = strings.ReplaceAll(value, "\x00", "")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "" {
		return "/"
	}
	return cleaned
}

func JoinRemotePath(parent, name string) string {
	parent = CleanRemotePath(parent)
	name = strings.TrimSpace(name)
	if parent == "/" {
		return "/" + name
	}
	return path.Join(parent, name)
}

func cleanUploadFilename(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return "", fmt.Errorf("invalid filename")
	}
	return name, nil
}

func readFilePreview(reader io.Reader) (string, int, bool, error) {
	buffered := bufio.NewReader(reader)
	var builder strings.Builder
	lines := 0
	bytesRead := 0
	truncated := false

	for lines < filePreviewLines && bytesRead < filePreviewMaxBytes {
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			remaining := filePreviewMaxBytes - bytesRead
			if len(line) > remaining {
				builder.WriteString(line[:remaining])
				bytesRead += remaining
				lines++
				truncated = true
				break
			}
			builder.WriteString(line)
			bytesRead += len(line)
			lines++
		}

		if err == nil {
			continue
		}
		if err == io.EOF {
			break
		}
		return "", 0, false, err
	}

	if !truncated && (lines == filePreviewLines || bytesRead == filePreviewMaxBytes) {
		if _, err := buffered.Peek(1); err == nil {
			truncated = true
		}
	}

	return builder.String(), lines, truncated, nil
}
