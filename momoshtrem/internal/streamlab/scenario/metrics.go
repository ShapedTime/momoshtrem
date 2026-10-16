package scenario

import "github.com/shapedtime/momoshtrem/internal/streamlab/report"

// Metrics describes every metric a repetition can report (PLAN §6
// "Metric kinds"). A scenario reports only those it measures.
var Metrics = []report.MetricDef{
	// Exact: deterministic; any change is reported.
	{Name: "integrity_errors", Kind: report.Exact, Unit: "bytes", Better: report.Lower, Doc: "Bytes received that didn't match the media."},
	{Name: "panics", Kind: report.Exact, Unit: "panics", Better: report.Lower, Doc: "Handler panics net/http recovered."},
	{Name: "leaked_goroutines", Kind: report.Exact, Unit: "goroutines", Better: report.Lower, Doc: "Goroutines carrying a request's label left after quiescence (not the anacrolix ones a request started, which live with the torrent)."},
	{Name: "readers_left", Kind: report.Exact, Unit: "readers", Better: report.Lower, Doc: "PriorityReaders still registered after quiescence."},
	{Name: "requests_left", Kind: report.Exact, Unit: "requests", Better: report.Lower, Doc: "Requests whose handler was still running after quiescence."},
	{Name: "open_files_left", Kind: report.Exact, Unit: "files", Better: report.Lower, Doc: "The open_files gauge after quiescence."},
	{Name: "blocked_reads_left", Kind: report.Exact, Unit: "reads", Better: report.Lower, Doc: "The blocked_reads gauge after quiescence."},
	{Name: "sessions_left", Kind: report.Exact, Unit: "sessions", Better: report.Lower, Doc: "Streaming sessions left after the sessions clock was skewed 31 s."},
	{Name: "readers_created", Kind: report.Exact, Unit: "readers", Better: report.None, Doc: "PriorityReaders created: priority_updates_total{type=initial}, one InitialPrioritize each."},
	{Name: "torrents_loaded_by_propfind", Kind: report.Exact, Unit: "torrents", Better: report.Lower, Doc: "Torrents a Depth 1 PROPFIND loaded (F3)."},
	{Name: "sniff_readers", Kind: report.Exact, Unit: "readers", Better: report.Lower, Doc: "Readers created by PROPFINDs and HEADs: content sniffed at offset 0 (F1)."},
	{Name: "etag_changes", Kind: report.Exact, Unit: "etags", Better: report.Lower, Doc: "Distinct ETags of one file over HEADs and PROPFINDs, less one (F2)."},
	{Name: "if_range_honoured", Kind: report.Exact, Unit: "bool", Better: report.Higher, Doc: "1 when a Range GET with the file's ETag as If-Range answered 206 (F2)."},
	{Name: "pieces_lowered_by_close", Kind: report.Exact, Unit: "pieces", Better: report.Lower, Doc: "Incomplete pieces of reader A's window whose effective priority dropped when reader B, open 30 s or more, closed (F6)."},
	{Name: "pieces_lowered_by_open", Kind: report.Exact, Unit: "pieces", Better: report.Lower, Doc: "Incomplete pieces of reader A's window whose effective priority dropped when reader B opened (F13)."},
	{Name: "evicted_complete_pieces", Kind: report.Exact, Unit: "pieces", Better: report.Lower, Doc: "Pieces anacrolix believes complete whose data the piece cache (the filecache LRU) has evicted (F11)."},

	// Count: flagged past a noise floor.
	{Name: "reads", Kind: report.Count, Unit: "reads", Better: report.None, Doc: "Streaming reads: reads_total."},
	{Name: "bytes_served", Kind: report.Count, Unit: "bytes", Better: report.None, Doc: "Body bytes the WebDAV server sent for GETs."},
	{Name: "bytes_downloaded", Kind: report.Count, Unit: "bytes", Better: report.Lower, Doc: "Data bytes the leecher downloaded from peers."},
	{Name: "waste_ratio", Kind: report.Count, Unit: "ratio", Better: report.Lower, Doc: "bytes_downloaded over bytes_served."},
	{Name: "residual_download_bytes", Kind: report.Count, Unit: "bytes", Better: report.Lower, Doc: "Bytes downloaded after the last request closed: through the quiescence wait and residual_window more."},
	{Name: "seek_updates_per_mib", Kind: report.Count, Unit: "updates/MiB", Better: report.Lower, Doc: "priority_updates_total{type=seek} per MiB read through the VFS."},
	{Name: "debounced_updates_per_mib", Kind: report.Count, Unit: "updates/MiB", Better: report.None, Doc: "priority_updates_total{type=seek_debounced} per MiB read through the VFS."},
	{Name: "seeks_actual", Kind: report.Count, Unit: "seeks", Better: report.None, Doc: "Reader seeks that moved: seek_actual_total."},
	{Name: "seeks_redundant", Kind: report.Count, Unit: "seeks", Better: report.Lower, Doc: "Reader seeks that didn't move: seek_redundant_total."},
	{Name: "pieces_downgraded", Kind: report.Count, Unit: "pieces", Better: report.None, Doc: "Completed pieces the prioritizer set back to None: pieces_downgraded_total."},
	{Name: "read_timeouts", Kind: report.Count, Unit: "reads", Better: report.Lower, Doc: "Server reads that timed out: read_timeouts_total."},
	{Name: "slow_reads", Kind: report.Count, Unit: "reads", Better: report.Lower, Doc: "Server reads that blocked over 500 ms: slow_reads_total."},
	{Name: "over_read_bytes", Kind: report.Count, Unit: "bytes", Better: report.Lower, Doc: "Bytes the players received and never played: their buffers when they seeked or stopped."},
	{Name: "next_header_prefetched_bytes", Kind: report.Count, Unit: "bytes", Better: report.Higher, Doc: "Bytes of the next episode's header complete when it opened: the prefetch at Normal (3907536)."},

	// Timing: judged only by an interleaved A/B.
	{Name: "ttfb", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The first playback's time from its GET to its first byte."},
	{Name: "time_to_playable", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The first playback's time from its GET to playable_bytes buffered."},
	{Name: "seek_latency_p50", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "Median time from a seek to playable again."},
	{Name: "seek_latency_p95", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "95th percentile time from a seek to playable again."},
	{Name: "stall_count", Kind: report.Timing, Unit: "stalls", Better: report.Lower, Doc: "Times a player's buffer ran empty while playing."},
	{Name: "stall_time", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "Time players spent stalled."},
	{Name: "rebuffer_ratio", Kind: report.Timing, Unit: "ratio", Better: report.Lower, Doc: "stall_time over stall_time plus playing time."},
	{Name: "stall_time_after", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "Reader A's stall time after reader B opened or closed (F6, F13)."},
	{Name: "next_time_to_playable", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The next episode's time from its GET to playable."},
	{Name: "resume_ttfb", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "After a pause past the idle timeout, the time from resuming to the next byte."},
	{Name: "activation_wait", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "Mean time waitForFirstAccess blocked: activation_wait_seconds."},
	{Name: "handler_lag_after_abort", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The longest a handler ran after its client dropped the request: server end minus client abort (F12)."},
	{Name: "propfind_latency", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The slowest PROPFIND."},
	{Name: "probe_latency", Kind: report.Timing, Unit: "ms", Better: report.Lower, Doc: "The slowest header or footer probe."},
	{Name: "wall_time", Kind: report.Timing, Unit: "ms", Better: report.None, Doc: "The script's wall time."},
}

// LookupMetric returns the metric's description.
func LookupMetric(name string) (report.MetricDef, bool) {
	for _, m := range Metrics {
		if m.Name == name {
			return m, true
		}
	}
	return report.MetricDef{}, false
}
