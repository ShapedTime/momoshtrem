package common

import (
	"io/fs"
	"time"
)

// FileInfo implements os.FileInfo for virtual files.
type FileInfo struct {
	FileName    string
	FileSize    int64
	FileIsDir   bool
	FileModTime time.Time
}

func (fi *FileInfo) Name() string       { return fi.FileName }
func (fi *FileInfo) Size() int64        { return fi.FileSize }
func (fi *FileInfo) IsDir() bool        { return fi.FileIsDir }
func (fi *FileInfo) ModTime() time.Time { return fi.FileModTime }
func (fi *FileInfo) Sys() interface{}   { return nil }

func (fi *FileInfo) Mode() fs.FileMode {
	if fi.FileIsDir {
		return fs.ModeDir | 0755
	}
	return 0644
}

// NewFileInfo creates a new FileInfo with the given parameters.
func NewFileInfo(name string, size int64, isDir bool, modTime time.Time) *FileInfo {
	return &FileInfo{
		FileName:    name,
		FileSize:    size,
		FileIsDir:   isDir,
		FileModTime: modTime,
	}
}
