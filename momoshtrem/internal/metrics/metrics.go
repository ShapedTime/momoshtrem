package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds streaming I/O metrics for direct instrumentation in the VFS layer.
type Metrics struct {
	StreamingReadBytes    prometheus.Counter
	StreamingReads        prometheus.Counter
	StreamingReadTimeouts prometheus.Counter
	StreamingReadDuration prometheus.Histogram
	StreamingOpenFiles    prometheus.Gauge

	// Streaming performance diagnostics
	StreamingSeeks            *prometheus.CounterVec // labels: direction=forward|backward
	StreamingPiecesDowngraded prometheus.Counter
	StreamingSlowReads        prometheus.Counter

	// Seek redundancy tracking (hypothesis 3: redundant WebDAV seeks)
	StreamingSeekRedundant prometheus.Counter // Seeks where offset matched current reader position
	StreamingSeekActual    prometheus.Counter // Seeks that actually moved the reader position

	// Priority update frequency (hypotheses 2 & 3)
	StreamingPriorityUpdates *prometheus.CounterVec // labels: type=initial|seek|seek_debounced|format
}

// New creates and registers streaming metrics with the given registry.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		StreamingReadBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "read_bytes_total",
			Help:      "Total bytes read through streaming VFS.",
		}),
		StreamingReads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "reads_total",
			Help:      "Total streaming read operations.",
		}),
		StreamingReadTimeouts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "read_timeouts_total",
			Help:      "Streaming reads that timed out.",
		}),
		StreamingReadDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "read_duration_seconds",
			Help:      "Duration of streaming read operations.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}),
		StreamingOpenFiles: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "open_files",
			Help:      "Number of currently open torrent file handles.",
		}),
		StreamingSeeks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "seek_total",
			Help:      "Seek operations by direction. High backward rate indicates rebuffering.",
		}, []string{"direction"}),
		StreamingPiecesDowngraded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "pieces_downgraded_total",
			Help:      "Pieces downgraded from high to normal priority (behind playback cursor).",
		}),
		StreamingSlowReads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "slow_reads_total",
			Help:      "Reads that blocked over 500ms waiting for piece data.",
		}),
		StreamingSeekRedundant: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "seek_redundant_total",
			Help:      "Seeks where offset matched current reader position (wasted work).",
		}),
		StreamingSeekActual: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "seek_actual_total",
			Help:      "Seeks that actually moved the reader position.",
		}),
		StreamingPriorityUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "momoshtrem",
			Subsystem: "streaming",
			Name:      "priority_updates_total",
			Help:      "Priority update operations by type.",
		}, []string{"type"}),
	}

	reg.MustRegister(
		m.StreamingReadBytes,
		m.StreamingReads,
		m.StreamingReadTimeouts,
		m.StreamingReadDuration,
		m.StreamingOpenFiles,
		m.StreamingSeeks,
		m.StreamingPiecesDowngraded,
		m.StreamingSlowReads,
		m.StreamingSeekRedundant,
		m.StreamingSeekActual,
		m.StreamingPriorityUpdates,
	)

	return m
}
