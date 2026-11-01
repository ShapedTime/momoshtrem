package report

// LoadAvg is the machine's 1, 5 and 15 minute load averages, best
// effort: nil when they can't be read. streamlab ab samples it before
// every repetition.
func LoadAvg() []float64 { return loadAvg() }
