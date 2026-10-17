package storagecap

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// ErrNotAnObject is a PUT body that isn't a JSON object (400).
var ErrNotAnObject = errors.New("send the settings to change as a JSON object")

// FieldError is a bad value in a PUT (422): Field is its JSON path, e.g.
// "kept.max_bytes".
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// CommittedError refuses a change that would cap kept downloads below what
// they already hold or want (409 limit_below_committed).
type CommittedError struct {
	// CommittedKeptBytes is the wanted size of every kept download.
	CommittedKeptBytes int64
	// KeptMaxBytes is the kept cap the change would leave.
	KeptMaxBytes int64
}

func (e *CommittedError) Error() string {
	return fmt.Sprintf("Kept downloads need %s, but this leaves them %s. Free some kept downloads first.",
		formatBytes(e.CommittedKeptBytes), formatBytes(e.KeptMaxBytes))
}

// ParsePatch reads a PUT /settings/storage body: only the fields sent
// change, and the nested objects merge. Unknown fields are ignored. Don't
// send the GET shape back as it is: GET answers limit_bytes null while the
// limit comes from config.yaml, and a PUT of null is auto. Send limit_bytes
// only when the limit control changes. It answers ErrNotAnObject, or a
// *FieldError for the first value of the wrong type or out of range; the
// checks that depend on the disk are UpdateStorage's.
func ParsePatch(raw []byte) (library.StorageSettingsPatch, error) {
	var p library.StorageSettingsPatch
	top, ok := object(raw)
	if !ok {
		return p, ErrNotAnObject
	}
	if v, sent := top["limit_bytes"]; sent {
		p.SetLimit = true
		var err *FieldError
		if p.LimitBytes, err = bytesField("limit_bytes", v, "or null for auto"); err != nil {
			return p, err
		}
	}
	if err := parseKept(top, &p.Kept); err != nil {
		return p, err
	}
	if err := parseSmart(top, &p.Smart); err != nil {
		return p, err
	}
	if err := parseBackground(top, &p.Background); err != nil {
		return p, err
	}
	return p, nil
}

func parseKept(top map[string]json.RawMessage, p *library.KeptSettingsPatch) *FieldError {
	fields, err := nested(top, "kept")
	if fields == nil || err != nil {
		return err
	}
	if v, sent := fields["max_bytes"]; sent {
		p.SetMaxBytes = true
		if p.MaxBytes, err = bytesField("kept.max_bytes", v, "or null for half the limit"); err != nil {
			return err
		}
	}
	if v, sent := fields["default_until_watched"]; sent {
		if p.DefaultUntilWatched, err = boolField("kept.default_until_watched", v); err != nil {
			return err
		}
	}
	if v, sent := fields["default_days"]; sent {
		p.SetDefaultDays = true
		if string(v) != "null" {
			var days int
			if json.Unmarshal(v, &days) != nil || !slices.Contains(library.KeptDayChoices, days) {
				return &FieldError{Field: "kept.default_days", Message: "kept.default_days must be 7, 30 or 90, or null for no maximum"}
			}
			p.DefaultDays = &days
		}
	}
	return nil
}

func parseSmart(top map[string]json.RawMessage, p *library.SmartSettingsPatch) *FieldError {
	fields, err := nested(top, "smart")
	if fields == nil || err != nil {
		return err
	}
	if v, sent := fields["enabled"]; sent {
		if p.Enabled, err = boolField("smart.enabled", v); err != nil {
			return err
		}
	}
	if v, sent := fields["episodes_ahead"]; sent {
		var n int
		if json.Unmarshal(v, &n) != nil || n < library.MinEpisodesAhead || n > library.MaxEpisodesAhead {
			return &FieldError{Field: "smart.episodes_ahead", Message: fmt.Sprintf(
				"smart.episodes_ahead must be a whole number from %d to %d", library.MinEpisodesAhead, library.MaxEpisodesAhead)}
		}
		p.EpisodesAhead = &n
	}
	if v, sent := fields["max_bytes"]; sent {
		p.SetMaxBytes = true
		if p.MaxBytes, err = bytesField("smart.max_bytes", v, "or null for 15% of the limit"); err != nil {
			return err
		}
	}
	return nil
}

func parseBackground(top map[string]json.RawMessage, p *library.BackgroundSettingsPatch) *FieldError {
	fields, err := nested(top, "background")
	if fields == nil || err != nil {
		return err
	}
	if v, sent := fields["enabled"]; sent {
		if p.Enabled, err = boolField("background.enabled", v); err != nil {
			return err
		}
	}
	if v, sent := fields["pause_while_streaming"]; sent {
		if p.PauseWhileStreaming, err = boolField("background.pause_while_streaming", v); err != nil {
			return err
		}
	}
	if v, sent := fields["windows"]; sent {
		p.SetWindows = true
		if p.Windows, err = windowsField(v); err != nil {
			return err
		}
	}
	return nil
}

// windowsField: at most 4 windows of two different "HH:MM" times.
func windowsField(raw json.RawMessage) ([]library.DownloadWindow, *FieldError) {
	const field = "background.windows"
	var windows []library.DownloadWindow
	if string(raw) == "null" || json.Unmarshal(raw, &windows) != nil {
		return nil, &FieldError{Field: field, Message: field + ` must be a list of {"start": "HH:MM", "end": "HH:MM"}, or [] for any time`}
	}
	if len(windows) > library.MaxDownloadWindows {
		return nil, &FieldError{Field: field, Message: fmt.Sprintf("%s takes at most %d windows", field, library.MaxDownloadWindows)}
	}
	for i, w := range windows {
		_, startOK := library.ClockMinutes(w.Start)
		_, endOK := library.ClockMinutes(w.End)
		switch {
		case !startOK || !endOK:
			return nil, &FieldError{Field: field, Message: fmt.Sprintf("%s: window %d needs a start and an end from 00:00 to 23:59", field, i+1)}
		case w.Start == w.End:
			return nil, &FieldError{Field: field, Message: fmt.Sprintf("%s: window %d starts when it ends", field, i+1)}
		}
	}
	if windows == nil {
		windows = []library.DownloadWindow{}
	}
	return windows, nil
}

// checkLimits is the PUT's checks that depend on the disk and the limit:
// merged is the settings with the patch applied, next their limits. A PUT
// that sends the limit or a cap must leave every explicit cap fitting,
// whether it was stored before or sent now, so the answer doesn't depend
// on the order of PUTs. The error names the field the PUT sent: the cap
// that doesn't fit, else the one that squeezed it. Defaults always fit.
func checkLimits(p library.StorageSettingsPatch, merged library.StorageSettings, next Limits) *FieldError {
	if p.SetLimit && p.LimitBytes != nil {
		if v := *p.LimitBytes; v < MinBytes || (next.MaxBytes > 0 && v > next.MaxBytes) {
			return &FieldError{Field: "limit_bytes", Message: fmt.Sprintf(
				"limit_bytes must be from %s to %s (90%% of the disk), or null for auto", formatBytes(MinBytes), formatBytes(next.MaxBytes))}
		}
	}
	sentLimit, sentKept, sentSmart := p.SetLimit, p.Kept.SetMaxBytes, p.Smart.SetMaxBytes
	if !sentLimit && !sentKept && !sentSmart {
		return nil
	}
	room := next.LimitBytes - next.ReserveBytes
	if kept := merged.Kept.MaxBytes; kept != nil && *kept > room {
		switch {
		case sentKept:
			return &FieldError{Field: "kept.max_bytes", Message: fmt.Sprintf(
				"kept.max_bytes can be at most %s: the limit less the streaming reserve", formatBytes(room))}
		case sentLimit:
			return &FieldError{Field: "limit_bytes", Message: fmt.Sprintf(
				"This limit leaves kept downloads %s, below their cap of %s: lower kept.max_bytes too", formatBytes(room), formatBytes(*kept))}
		}
		// Only smart sent: kept leaves it nothing, checked below.
	}
	left := room - next.KeptMaxBytes
	if smart := merged.Smart.MaxBytes; smart != nil && *smart > left {
		switch {
		case sentSmart:
			return &FieldError{Field: "smart.max_bytes", Message: fmt.Sprintf(
				"smart.max_bytes can be at most %s: what kept downloads and the streaming reserve leave of the limit", formatBytes(left))}
		case sentKept:
			return &FieldError{Field: "kept.max_bytes", Message: fmt.Sprintf(
				"This kept cap leaves smart downloads %s, below their cap of %s: lower smart.max_bytes too, or set it to null", formatBytes(left), formatBytes(*smart))}
		default:
			return &FieldError{Field: "limit_bytes", Message: fmt.Sprintf(
				"This limit leaves smart downloads %s, below their cap of %s: lower smart.max_bytes too", formatBytes(left), formatBytes(*smart))}
		}
	}
	return nil
}

// object reads a JSON object; ok is false for anything else, null included.
func object(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

// nested reads the object sent at name: nil fields when it isn't sent.
func nested(top map[string]json.RawMessage, name string) (map[string]json.RawMessage, *FieldError) {
	raw, sent := top[name]
	if !sent {
		return nil, nil
	}
	fields, ok := object(raw)
	if !ok {
		return nil, &FieldError{Field: name, Message: name + " must be an object of the fields to change"}
	}
	return fields, nil
}

// bytesField: null, or a whole number of bytes, 0 or more.
func bytesField(name string, raw json.RawMessage, null string) (*int64, *FieldError) {
	if string(raw) == "null" {
		return nil, nil
	}
	var v int64
	if json.Unmarshal(raw, &v) != nil || v < 0 {
		return nil, &FieldError{Field: name, Message: name + " must be a whole number of bytes, " + null}
	}
	return &v, nil
}

func boolField(name string, raw json.RawMessage) (*bool, *FieldError) {
	var v bool
	if string(raw) == "null" || json.Unmarshal(raw, &v) != nil {
		return nil, &FieldError{Field: name, Message: name + " must be true or false"}
	}
	return &v, nil
}

// formatBytes writes a size as the web's formatBytes does
// (web/src/lib/title/format.ts), so a message reads like the screen that
// shows it: decimal units, one decimal below 100, none from 100 or in
// bytes. 4 GiB is "4.3 GB".
func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v, unit := float64(max(n, 0)), 0
	for v >= 1000 && unit < len(units)-1 {
		v /= 1000
		unit++
	}
	digits := 1
	if unit == 0 || v >= 100 {
		digits = 0
	}
	return toFixed(v, digits) + " " + units[unit]
}

// toFixed is JavaScript's Number.prototype.toFixed: the nearest, and on an
// exact tie the larger, where strconv takes the even one (2.25 is "2.3").
func toFixed(v float64, digits int) string {
	scale := math.Pow10(digits)
	scaled := v * scale
	if _, frac := math.Modf(scaled); frac == 0.5 && math.FMA(v, scale, -scaled) == 0 {
		return strconv.FormatFloat(math.Ceil(scaled)/scale, 'f', digits, 64)
	}
	return strconv.FormatFloat(v, 'f', digits, 64)
}
