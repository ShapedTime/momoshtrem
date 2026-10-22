// Package member is the household's members (#239; PLAN §6 "Household
// members"): up to six people sharing the one login, each with their own
// Trakt, picks and Ask. A request is for one member, named by the
// X-Momoshtrem-Member header; the API's middleware puts it in the request
// context (WithID), and the top-level service methods read it (Require).
// Below them ids are explicit: repositories take a memberID, or are bound
// to one, and never read the context.
//
// It imports nothing of momoshtrem's, so library implements its Store.
package member

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxMembers is the household's cap.
const MaxMembers = 6

// MaxNameLen is a name's longest, in characters (code points) once
// trimmed.
const MaxNameLen = 32

// Colors are the palette's keys, in the order a new member's default is
// picked (the web maps each to a token). The first member is teal.
var Colors = []string{"rose", "orchid", "periwinkle", "teal", "lime", "sand"}

// Member is one person in the household.
type Member struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// Patch changes the fields set.
type Patch struct {
	Name  *string
	Color *string
}

var (
	// ErrNoMember: the context names no member (Require).
	ErrNoMember = errors.New("no household member in the context")
	// ErrMalformed: the header isn't a positive base-10 integer.
	ErrMalformed = errors.New("the member header isn't a member id")
	// ErrUnknown: the header names an id no member has.
	ErrUnknown = errors.New("no household member has that id")
	// ErrNotFound: the member to change or remove doesn't exist.
	ErrNotFound = errors.New("member not found")
	// ErrLimit: the household has MaxMembers already.
	ErrLimit = errors.New("the household is full")
	// ErrNameTaken: another member has the name, ignoring case.
	ErrNameTaken = errors.New("another member has that name")
	// ErrLastMember: the household can't be left with nobody.
	ErrLastMember = errors.New("the last member can't be removed")
)

// ValidationError is a bad value, naming its JSON field (name or color).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

type ctxKey struct{}

// WithID returns ctx for the member id.
func WithID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IDFrom is the context's member, if it names one.
func IDFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxKey{}).(int64)
	return id, ok
}

// Require is the context's member, or ErrNoMember.
func Require(ctx context.Context) (int64, error) {
	if id, ok := IDFrom(ctx); ok {
		return id, nil
	}
	return 0, ErrNoMember
}

// ParseID reads a header value: a positive base-10 integer, digits only.
func ParseID(header string) (int64, error) {
	if header == "" || strings.Trim(header, "0123456789") != "" {
		return 0, ErrMalformed
	}
	id, err := strconv.ParseInt(header, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrMalformed
	}
	return id, nil
}

// CleanName trims a name and checks its length.
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameLen {
		return "", &ValidationError{Field: "name", Message: "name must be 1 to 32 characters"}
	}
	return name, nil
}

// CheckColor checks a colour is one of Colors.
func CheckColor(color string) error {
	if !slices.Contains(Colors, color) {
		last := len(Colors) - 1
		return &ValidationError{Field: "color", Message: "color must be " + strings.Join(Colors[:last], ", ") + " or " + Colors[last]}
	}
	return nil
}

// DefaultColor is the first of Colors no member uses (used), else the
// first.
func DefaultColor(used []string) string {
	for _, c := range Colors {
		if !slices.Contains(used, c) {
			return c
		}
	}
	return Colors[0]
}
