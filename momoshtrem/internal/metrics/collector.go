package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// TorrentCollector implements prometheus.Collector for torrent stats.
// It polls torrent.Service.CollectStats() lazily on each Prometheus scrape
// rather than maintaining duplicate state.
type TorrentCollector struct {
	service  torrent.Service
	activity *torrent.ActivityManager // may be nil

	// Per-torrent descriptors (labels: info_hash, name)
	sizeBytes        *prometheus.Desc
	bytesCompleted   *prometheus.Desc
	progressRatio    *prometheus.Desc
	peersActive      *prometheus.Desc
	seedersConnected *prometheus.Desc
	peersHalfOpen    *prometheus.Desc
	piecesComplete   *prometheus.Desc
	downloadedTotal  *prometheus.Desc
	uploadedTotal    *prometheus.Desc
	chunksWasted     *prometheus.Desc
	piecesVerified   *prometheus.Desc
	piecesFailed     *prometheus.Desc

	// Aggregate descriptors (no per-torrent labels)
	torrentsLoaded *prometheus.Desc
	torrentsActive *prometheus.Desc
	torrentsIdle   *prometheus.Desc
}

var torrentLabels = []string{"info_hash", "name"}

// NewTorrentCollector creates a collector that scrapes torrent stats on demand.
func NewTorrentCollector(svc torrent.Service, am *torrent.ActivityManager) *TorrentCollector {
	return &TorrentCollector{
		service:  svc,
		activity: am,

		sizeBytes: prometheus.NewDesc(
			"momoshtrem_torrent_size_bytes",
			"Total size of the torrent in bytes.",
			torrentLabels, nil,
		),
		bytesCompleted: prometheus.NewDesc(
			"momoshtrem_torrent_bytes_completed",
			"Bytes completed (downloaded and verified) for the torrent.",
			torrentLabels, nil,
		),
		progressRatio: prometheus.NewDesc(
			"momoshtrem_torrent_progress_ratio",
			"Download progress as a ratio from 0.0 to 1.0.",
			torrentLabels, nil,
		),
		peersActive: prometheus.NewDesc(
			"momoshtrem_torrent_peers_active",
			"Number of actively transferring peers.",
			torrentLabels, nil,
		),
		seedersConnected: prometheus.NewDesc(
			"momoshtrem_torrent_seeders_connected",
			"Number of connected seeders.",
			torrentLabels, nil,
		),
		peersHalfOpen: prometheus.NewDesc(
			"momoshtrem_torrent_peers_half_open",
			"Number of half-open (connecting) peers.",
			torrentLabels, nil,
		),
		piecesComplete: prometheus.NewDesc(
			"momoshtrem_torrent_pieces_complete",
			"Number of fully downloaded pieces.",
			torrentLabels, nil,
		),
		downloadedTotal: prometheus.NewDesc(
			"momoshtrem_torrent_downloaded_bytes_total",
			"Total data bytes downloaded from peers.",
			torrentLabels, nil,
		),
		uploadedTotal: prometheus.NewDesc(
			"momoshtrem_torrent_uploaded_bytes_total",
			"Total data bytes uploaded to peers.",
			torrentLabels, nil,
		),
		chunksWasted: prometheus.NewDesc(
			"momoshtrem_torrent_chunks_wasted_total",
			"Total wasted chunks received (duplicates or unwanted).",
			torrentLabels, nil,
		),
		piecesVerified: prometheus.NewDesc(
			"momoshtrem_torrent_pieces_verified_total",
			"Total pieces that passed hash verification.",
			torrentLabels, nil,
		),
		piecesFailed: prometheus.NewDesc(
			"momoshtrem_torrent_pieces_failed_total",
			"Total pieces that failed hash verification.",
			torrentLabels, nil,
		),

		torrentsLoaded: prometheus.NewDesc(
			"momoshtrem_torrents_loaded",
			"Total number of loaded torrents.",
			nil, nil,
		),
		torrentsActive: prometheus.NewDesc(
			"momoshtrem_torrents_active",
			"Number of active (non-idle) torrents.",
			nil, nil,
		),
		torrentsIdle: prometheus.NewDesc(
			"momoshtrem_torrents_idle",
			"Number of idle (paused) torrents.",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *TorrentCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sizeBytes
	ch <- c.bytesCompleted
	ch <- c.progressRatio
	ch <- c.peersActive
	ch <- c.seedersConnected
	ch <- c.peersHalfOpen
	ch <- c.piecesComplete
	ch <- c.downloadedTotal
	ch <- c.uploadedTotal
	ch <- c.chunksWasted
	ch <- c.piecesVerified
	ch <- c.piecesFailed
	ch <- c.torrentsLoaded
	ch <- c.torrentsActive
	ch <- c.torrentsIdle
}

// Collect implements prometheus.Collector.
func (c *TorrentCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.service.CollectStats()

	for _, s := range stats {
		labels := []string{s.InfoHash, s.Name}

		var progress float64
		if s.TotalSize > 0 {
			progress = float64(s.BytesCompleted) / float64(s.TotalSize)
		}

		ch <- prometheus.MustNewConstMetric(c.sizeBytes, prometheus.GaugeValue, float64(s.TotalSize), labels...)
		ch <- prometheus.MustNewConstMetric(c.bytesCompleted, prometheus.GaugeValue, float64(s.BytesCompleted), labels...)
		ch <- prometheus.MustNewConstMetric(c.progressRatio, prometheus.GaugeValue, progress, labels...)
		ch <- prometheus.MustNewConstMetric(c.peersActive, prometheus.GaugeValue, float64(s.ActivePeers), labels...)
		ch <- prometheus.MustNewConstMetric(c.seedersConnected, prometheus.GaugeValue, float64(s.ConnectedSeeders), labels...)
		ch <- prometheus.MustNewConstMetric(c.peersHalfOpen, prometheus.GaugeValue, float64(s.HalfOpenPeers), labels...)
		ch <- prometheus.MustNewConstMetric(c.piecesComplete, prometheus.GaugeValue, float64(s.PiecesComplete), labels...)
		ch <- prometheus.MustNewConstMetric(c.downloadedTotal, prometheus.CounterValue, float64(s.BytesReadData), labels...)
		ch <- prometheus.MustNewConstMetric(c.uploadedTotal, prometheus.CounterValue, float64(s.BytesWrittenData), labels...)
		ch <- prometheus.MustNewConstMetric(c.chunksWasted, prometheus.CounterValue, float64(s.ChunksReadWasted), labels...)
		ch <- prometheus.MustNewConstMetric(c.piecesVerified, prometheus.CounterValue, float64(s.PiecesDirtiedGood), labels...)
		ch <- prometheus.MustNewConstMetric(c.piecesFailed, prometheus.CounterValue, float64(s.PiecesDirtiedBad), labels...)
	}

	// Aggregate metrics
	ch <- prometheus.MustNewConstMetric(c.torrentsLoaded, prometheus.GaugeValue, float64(len(stats)))

	if c.activity != nil {
		activityStats := c.activity.GetStats()
		if v, ok := activityStats["active_torrents"].(int); ok {
			ch <- prometheus.MustNewConstMetric(c.torrentsActive, prometheus.GaugeValue, float64(v))
		}
		if v, ok := activityStats["idle_torrents"].(int); ok {
			ch <- prometheus.MustNewConstMetric(c.torrentsIdle, prometheus.GaugeValue, float64(v))
		}
	} else {
		ch <- prometheus.MustNewConstMetric(c.torrentsActive, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.torrentsIdle, prometheus.GaugeValue, 0)
	}
}
