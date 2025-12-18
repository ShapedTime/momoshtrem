//go:build !fuse

package fuse

import (
	"github.com/distribyted/distribyted/fs"
	"github.com/rs/zerolog/log"
)

// Handler is a stub when FUSE is not available
type Handler struct {
	path string
}

// NewHandler creates a stub handler when FUSE is not available
func NewHandler(fuseAllowOther bool, path string) *Handler {
	return &Handler{
		path: path,
	}
}

// Mount logs a warning that FUSE is not available
func (s *Handler) Mount(fss map[string]fs.Filesystem) error {
	log.Warn().Str("path", s.path).Msg("FUSE mount requested but FUSE support is not compiled in. Build with -tags=fuse to enable.")
	return nil
}

// Unmount is a no-op when FUSE is not available
func (s *Handler) Unmount() {
}
