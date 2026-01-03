package webdav

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/webdav"

	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// Server wraps a WebDAV server
type Server struct {
	fs      *vfs.LibraryFS
	handler *webdav.Handler
}

// NewServer creates a new WebDAV server
func NewServer(libraryFS *vfs.LibraryFS) *Server {
	s := &Server{fs: libraryFS}

	s.handler = &webdav.Handler{
		Prefix:     "",
		FileSystem: &webdavFS{fs: libraryFS},
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				slog.Debug("WebDAV request",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
			} else {
				slog.Debug("WebDAV request",
					"method", r.Method,
					"path", r.URL.Path,
				)
			}
		},
	}

	return s
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.handler
}

// webdavFS adapts LibraryFS to webdav.FileSystem
type webdavFS struct {
	fs *vfs.LibraryFS
}

func (wfs *webdavFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission // Read-only filesystem
}

func (wfs *webdavFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	// Reject write operations
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, os.ErrPermission
	}

	name = cleanPath(name)

	file, err := wfs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	return &webdavFile{
		file: file,
		fs:   wfs.fs,
		path: name,
	}, nil
}

func (wfs *webdavFS) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission // Read-only filesystem
}

func (wfs *webdavFS) Rename(ctx context.Context, oldName, newName string) error {
	return os.ErrPermission // Read-only filesystem
}

func (wfs *webdavFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = cleanPath(name)

	file, err := wfs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// webdavFile adapts vfs.File to webdav.File
type webdavFile struct {
	mu   sync.Mutex
	file vfs.File
	fs   *vfs.LibraryFS
	path string
	pos  int64

	// For directory listing
	dirMu      sync.Mutex
	dirEntries []os.FileInfo
	dirPos     int
}

func (f *webdavFile) Close() error {
	return f.file.Close()
}

func (f *webdavFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.file.ReadAt(p, f.pos)
	f.pos += int64(n)
	return n, err
}

func (f *webdavFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	size := f.file.Size()

	switch whence {
	case 0: // SeekStart
		f.pos = offset
	case 1: // SeekCurrent
		f.pos += offset
	case 2: // SeekEnd
		f.pos = size + offset
	}

	if f.pos < 0 {
		f.pos = 0
	}
	if f.pos > size {
		f.pos = size
	}

	return f.pos, nil
}

func (f *webdavFile) Readdir(count int) ([]os.FileInfo, error) {
	f.dirMu.Lock()
	defer f.dirMu.Unlock()

	if !f.file.IsDir() {
		return nil, os.ErrInvalid
	}

	// Load directory entries on first call
	if f.dirEntries == nil {
		entries, err := f.fs.ReadDir(f.path)
		if err != nil {
			return nil, err
		}

		for _, file := range entries {
			info, err := file.Stat()
			if err != nil {
				continue
			}
			f.dirEntries = append(f.dirEntries, info)
		}

		// Sort by name
		sort.Slice(f.dirEntries, func(i, j int) bool {
			return f.dirEntries[i].Name() < f.dirEntries[j].Name()
		})
	}

	// Return entries
	if count <= 0 {
		// Return all remaining
		entries := f.dirEntries[f.dirPos:]
		f.dirPos = len(f.dirEntries)
		return entries, nil
	}

	// Return up to count entries
	end := f.dirPos + count
	if end > len(f.dirEntries) {
		end = len(f.dirEntries)
	}

	entries := f.dirEntries[f.dirPos:end]
	f.dirPos = end

	return entries, nil
}

func (f *webdavFile) Stat() (os.FileInfo, error) {
	return f.file.Stat()
}

func (f *webdavFile) Write(p []byte) (int, error) {
	return 0, os.ErrPermission // Read-only
}

// ContentType returns the MIME type for the file
func (f *webdavFile) ContentType(ctx context.Context) (string, error) {
	if f.file.IsDir() {
		return "text/html; charset=utf-8", nil
	}

	// Determine content type from extension
	ext := strings.ToLower(path.Ext(f.file.Name()))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4", nil
	case ".mkv":
		return "video/x-matroska", nil
	case ".avi":
		return "video/x-msvideo", nil
	case ".mov":
		return "video/quicktime", nil
	case ".webm":
		return "video/webm", nil
	case ".srt":
		return "text/plain; charset=utf-8", nil
	case ".ass", ".ssa":
		return "text/plain; charset=utf-8", nil
	case ".vtt":
		return "text/vtt", nil
	case ".edl":
		return "text/plain", nil
	default:
		return "application/octet-stream", nil
	}
}

// ETag returns the entity tag for the file
func (f *webdavFile) ETag(ctx context.Context) (string, error) {
	// Use size and name as simple ETag
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	return `"` + info.Name() + "-" + itoa(info.Size()) + `"`, nil
}

// Implement DeadPropsHolder to satisfy webdav requirements
func (f *webdavFile) DeadProps() (map[string][]byte, error) {
	return nil, nil
}

func (f *webdavFile) Patch(patches []webdav.Proppatch) ([]webdav.Propstat, error) {
	return nil, os.ErrPermission
}

// Helper functions

func cleanPath(p string) string {
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string('0'+byte(n%10)) + s
		n /= 10
	}
	return s
}

// fileInfo wrapper for virtual files
type fileInfoWrapper struct {
	name    string
	size    int64
	isDir   bool
	modTime time.Time
}

func (fi *fileInfoWrapper) Name() string       { return fi.name }
func (fi *fileInfoWrapper) Size() int64        { return fi.size }
func (fi *fileInfoWrapper) Mode() fs.FileMode  {
	if fi.isDir {
		return fs.ModeDir | 0755
	}
	return 0644
}
func (fi *fileInfoWrapper) ModTime() time.Time { return fi.modTime }
func (fi *fileInfoWrapper) IsDir() bool        { return fi.isDir }
func (fi *fileInfoWrapper) Sys() interface{}   { return nil }
