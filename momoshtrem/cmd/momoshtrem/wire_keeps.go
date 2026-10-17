package main

import (
	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/downloads"
)

// The read paths show the downloads (#77 P6, #204) from the download
// manager's latest pass: title pages, /library and Up next through the
// catalog, Activity through its own reader. Each request reads one
// snapshot; before the first pass it's nil, which answers nothing: nothing
// is kept or downloading yet.

// catalogDownloads is the catalog's (and /me's) view of the downloads.
func catalogDownloads(svc *downloads.Service) func() catalog.DownloadsSnapshot {
	return func() catalog.DownloadsSnapshot { return svc.Snapshot() }
}

// activityDownloads is Activity's view of the downloads.
func activityDownloads(svc *downloads.Service) func() activity.DownloadsSnapshot {
	return func() activity.DownloadsSnapshot { return svc.Snapshot() }
}
