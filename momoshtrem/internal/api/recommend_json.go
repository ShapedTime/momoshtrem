package api

import "github.com/shapedtime/momoshtrem/internal/recommend"

// The AI routes' lists are always arrays in the contract (PLAN §6
// "Recommendations (R1–R3)"), but a nil Go slice marshals as null. These
// copy a value with every nil list made empty, so a service never has to
// remember to.

func orEmpty[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

func emptyProgressSlices(p recommend.RunProgress) recommend.RunProgress {
	p.Steps = orEmpty(p.Steps)
	return p
}

func emptyHomeSlices(h recommend.Home) recommend.Home {
	h.HeroPicks = orEmpty(h.HeroPicks)
	shelves := make([]recommend.Shelf, len(h.Shelves))
	for i, shelf := range h.Shelves {
		shelf.Items = orEmpty(shelf.Items)
		shelves[i] = shelf
	}
	h.Shelves = shelves
	if h.ActiveRun != nil {
		active := emptyProgressSlices(*h.ActiveRun)
		h.ActiveRun = &active
	}
	return h
}

func emptyTurnSlices(t recommend.Turn) recommend.Turn {
	t.Steps = orEmpty(t.Steps)
	t.Items = orEmpty(t.Items)
	t.Refinements = orEmpty(t.Refinements)
	return t
}

func emptyThreadSlices(t recommend.Thread) recommend.Thread {
	turns := make([]recommend.Turn, len(t.Turns))
	for i, turn := range t.Turns {
		turns[i] = emptyTurnSlices(turn)
	}
	t.Turns = turns
	return t
}

// emptyUsageSlices also gives estimates.run_usd a key per rec model (null
// before any run on it).
func emptyUsageSlices(u recommend.UsageReport) recommend.UsageReport {
	u.ByFeature = orEmpty(u.ByFeature)
	if u.Recs.ActiveRun != nil {
		active := emptyProgressSlices(*u.Recs.ActiveRun)
		u.Recs.ActiveRun = &active
	}
	runUSD := make(map[string]*float64, len(recommend.RecModels))
	for _, model := range recommend.RecModels {
		runUSD[model] = u.Estimates.RunUSD[model]
	}
	u.Estimates.RunUSD = runUSD
	return u
}

// emptyEventSlices fixes an SSE event's lists: done's refinements.
func emptyEventSlices(e recommend.Event) recommend.Event {
	switch data := e.Data.(type) {
	case recommend.DoneEvent:
		data.Refinements = orEmpty(data.Refinements)
		e.Data = data
	case *recommend.DoneEvent:
		if data != nil {
			done := *data
			done.Refinements = orEmpty(done.Refinements)
			e.Data = done
		}
	}
	return e
}
