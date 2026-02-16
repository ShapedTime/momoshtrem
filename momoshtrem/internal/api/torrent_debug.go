package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// getTorrentDebug returns detailed piece-level debug info for a torrent.
func (s *Server) getTorrentDebug(c *gin.Context) {
	hash := c.Param("hash")
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "torrent hash required")
		return
	}

	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "torrent service not available")
		return
	}

	debugInfo, err := s.torrentService.GetDebugInfo(hash)
	if err != nil {
		if err == torrent.ErrTorrentNotFound {
			errorResponse(c, http.StatusNotFound, "torrent not found")
			return
		}
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Populate readers from tracker if available
	if s.readerTracker != nil {
		snapshots := s.readerTracker.GetReaders(hash)
		readers := make([]torrent.DebugReaderInfo, len(snapshots))
		for i, snap := range snapshots {
			var posPiece int
			if debugInfo.PieceLength > 0 {
				posPiece = int(snap.Position / debugInfo.PieceLength)
			}
			readers[i] = torrent.DebugReaderInfo{
				FilePath:      snap.FilePath,
				Position:      snap.Position,
				PositionPiece: posPiece,
				FileLength:    snap.FileLength,
			}
		}
		debugInfo.Readers = readers
	}

	c.JSON(http.StatusOK, debugInfo)
}
