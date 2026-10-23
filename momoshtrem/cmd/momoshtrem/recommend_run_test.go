package main

import (
	"context"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/member"
)

func TestParseRecommendRunFlags(t *testing.T) {
	tests := []struct {
		args       []string
		wantDry    bool
		wantMember int64
		wantOK     bool
		wantCode   int
	}{
		{nil, false, 0, true, 0},
		{[]string{"-dry-run"}, true, 0, true, 0},
		{[]string{"--dry-run"}, true, 0, true, 0},
		{[]string{"-member", "2"}, false, 2, true, 0},
		{[]string{"-member=3", "-dry-run"}, true, 3, true, 0},
		{[]string{"-member", "0"}, false, 0, false, 2},
		{[]string{"-member", "-1"}, false, 0, false, 2},
		{[]string{"-member", "sam"}, false, 0, false, 2},
		{[]string{"-member"}, false, 0, false, 2},
		{[]string{"-h"}, false, 0, false, 0},
		{[]string{"-model", "x"}, false, 0, false, 2},
		{[]string{"extra"}, false, 0, false, 2},
	}
	for _, tt := range tests {
		f, code, ok := parseRecommendRunFlags(tt.args)
		if ok != tt.wantOK || code != tt.wantCode || (ok && (f.dryRun != tt.wantDry || f.member != tt.wantMember)) {
			t.Errorf("parse(%q) = %+v, %d, %v", tt.args, f, code, ok)
		}
	}
}

func TestStoredConnection(t *testing.T) {
	if !storedConnection(true).Connected() || storedConnection(false).Connected() {
		t.Error("storedConnection doesn't say what it holds")
	}
}

// membersStore is a household of Me (1) and Sam (3), read-only.
type membersStore struct{ member.Store }

func (membersStore) List(context.Context) ([]member.Member, error) {
	return []member.Member{{ID: 1, Name: "Me", Color: "teal"}, {ID: 3, Name: "Sam", Color: "rose"}}, nil
}

// -member picks the member; without it, the lowest.
func TestRecommendMember(t *testing.T) {
	members := member.NewService(membersStore{})
	ctx := context.Background()
	for id, want := range map[int64]string{0: "Me", 1: "Me", 3: "Sam"} {
		if m, err := recommendMember(ctx, members, id); err != nil || m.Name != want {
			t.Errorf("recommendMember(%d) = %+v, %v; want %s", id, m, err, want)
		}
	}
	if m, err := recommendMember(ctx, members, 2); err == nil {
		t.Errorf("recommendMember(2) = %+v; want an error, nobody has id 2", m)
	}
}
