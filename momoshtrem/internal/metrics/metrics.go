package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds streaming I/O metrics for direct instrumentation in the VFS layer.
type Metrics struct {
	StreamingReadBytes    prometheus.Counter
	StreamingReads        prometheus.Counter
	StreamingReadTimeouts prometheus.Counter
	StreamingReadDuration prometheus.Histogram
	StreamingOpenFiles    prometheus.Gauge
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
	}

	reg.MustRegister(
		m.StreamingReadBytes,
		m.StreamingReads,
		m.StreamingReadTimeouts,
		m.StreamingReadDuration,
		m.StreamingOpenFiles,
	)

	return m
}
