package vfs

import (
	"io"
	"os"
)

// Filesystem represents a virtual filesystem
type Filesystem interface {
	// Open opens a file for reading
	Open(path string) (File, error)

	// ReadDir returns the contents of a directory
	ReadDir(path string) (map[string]File, error)
}

// File represents a file in the virtual filesystem
type File interface {
	io.Reader
	io.ReaderAt
	io.Closer

	// Name returns the file name
	Name() string

	// IsDir returns true if this is a directory
	IsDir() bool

	// Size returns the file size in bytes
	Size() int64

	// Stat returns file info
	Stat() (os.FileInfo, error)
}
