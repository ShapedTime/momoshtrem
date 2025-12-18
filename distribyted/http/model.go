package http

import "github.com/distribyted/distribyted/torrent/loader"

type RouteAdd struct {
	Magnet   string               `json:"magnet" binding:"required"`
	Metadata *loader.TMDBMetadata `json:"metadata,omitempty"`
}

type MetadataUpdate struct {
	Metadata *loader.TMDBMetadata `json:"metadata" binding:"required"`
}

type TorrentInfo struct {
	Hash     string               `json:"hash"`
	Name     string               `json:"name"`
	Metadata *loader.TMDBMetadata `json:"metadata,omitempty"`
}

type Error struct {
	Error string `json:"error"`
}
