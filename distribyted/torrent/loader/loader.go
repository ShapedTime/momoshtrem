package loader

type Loader interface {
	ListMagnets() (map[string][]TorrentWithMetadata, error)
	ListTorrentPaths() (map[string][]string, error)
}

type LoaderAdder interface {
	Loader

	RemoveFromHash(r, h string) (bool, error)
	AddMagnet(r, m string, metadata *TMDBMetadata) error
	GetTorrentInfo(route, hash string) (*TorrentWithMetadata, error)
	UpdateMetadata(route, hash string, metadata *TMDBMetadata) error
}
