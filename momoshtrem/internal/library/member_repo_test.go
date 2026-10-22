package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/member"
)

func TestMemberRepository(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewMemberRepository(db)
	ctx := context.Background()

	// A new schema has one member: "Me" (no Trakt connection), teal.
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != 1 || list[0].Name != "Me" || list[0].Color != "teal" || list[0].CreatedAt.IsZero() {
		t.Fatalf("List = %+v, %v", list, err)
	}

	// No colour is the first nobody uses; a colour someone uses is fine.
	sam, err := repo.Create(ctx, "Sam", "")
	if err != nil || sam.ID != 2 || sam.Name != "Sam" || sam.Color != "rose" || sam.CreatedAt.IsZero() {
		t.Errorf("Create(Sam) = %+v, %v", sam, err)
	}
	if ari, err := repo.Create(ctx, "Ari", "teal"); err != nil || ari.Color != "teal" {
		t.Errorf("Create(Ari, teal) = %+v, %v", ari, err)
	}
	if m, err := repo.Create(ctx, "Kim", ""); err != nil || m.Color != "orchid" {
		t.Errorf("Create(Kim) = %+v, %v", m, err)
	}

	// Names are unique ignoring case.
	if _, err := repo.Create(ctx, "sAM", ""); !errors.Is(err, member.ErrNameTaken) {
		t.Errorf("Create(sAM) = %v, want ErrNameTaken", err)
	}
	// 32 characters fit and 33 don't, counted in code points: the CHECK
	// backs member.CleanName.
	if _, err := repo.Create(ctx, strings.Repeat("é", 33), ""); err == nil || errors.Is(err, member.ErrNameTaken) {
		t.Errorf("Create(33 characters) = %v, want the CHECK's error", err)
	}
	if _, err := repo.Create(ctx, strings.Repeat("é", 32), ""); err != nil {
		t.Errorf("Create(32 characters) = %v", err)
	}

	// Six at most.
	if _, err := repo.Create(ctx, "Lee", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, "Seventh", ""); !errors.Is(err, member.ErrLimit) {
		t.Errorf("a seventh = %v, want ErrLimit", err)
	}

	// Update: a member's own name in another case is fine, another's isn't.
	if m, err := repo.Update(ctx, 2, member.Patch{Name: strPtr("SAM")}); err != nil || m.Name != "SAM" || m.Color != "rose" || !m.CreatedAt.Equal(sam.CreatedAt) {
		t.Errorf("Update(SAM) = %+v, %v", m, err)
	}
	if _, err := repo.Update(ctx, 2, member.Patch{Name: strPtr("ari")}); !errors.Is(err, member.ErrNameTaken) {
		t.Errorf("Update(ari) = %v, want ErrNameTaken", err)
	}
	if m, err := repo.Update(ctx, 2, member.Patch{Color: strPtr("sand")}); err != nil || m.Name != "SAM" || m.Color != "sand" {
		t.Errorf("Update(sand) = %+v, %v", m, err)
	}
	var moved bool
	if err := db.QueryRow(`SELECT updated_at > created_at FROM members WHERE id = 2`).Scan(&moved); err != nil || !moved {
		t.Errorf("updated_at moved = %v (%v)", moved, err)
	}
	if _, err := repo.Update(ctx, 99, member.Patch{Color: strPtr("lime")}); !errors.Is(err, member.ErrNotFound) {
		t.Errorf("Update(99) = %v, want ErrNotFound", err)
	}

	// Delete: never the last one.
	if err := repo.Delete(ctx, 99); !errors.Is(err, member.ErrNotFound) {
		t.Errorf("Delete(99) = %v, want ErrNotFound", err)
	}
	list, _ = repo.List(ctx)
	for _, m := range list[1:] {
		if err := repo.Delete(ctx, m.ID); err != nil {
			t.Fatalf("Delete(%d) = %v", m.ID, err)
		}
	}
	if err := repo.Delete(ctx, 1); !errors.Is(err, member.ErrLastMember) {
		t.Errorf("Delete(the last) = %v, want ErrLastMember", err)
	}
	// A removed member's id is never handed out again.
	if m, err := repo.Create(ctx, "Sam", ""); err != nil || m.ID <= list[len(list)-1].ID || m.Color != "rose" {
		t.Errorf("Create after the deletes = %+v, %v", m, err)
	}
}

// The cap and "never zero members" hold under concurrent calls: Create
// and Delete hold the members' lock.
func TestMemberRepository_Concurrent(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewMemberRepository(db)
	ctx := context.Background()
	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM members`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Twelve creates at once: five fit beside "Me".
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = repo.Create(ctx, fmt.Sprintf("Person %d", i), "")
		}()
	}
	wg.Wait()
	created, full := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, member.ErrLimit):
			full++
		default:
			t.Errorf("Create = %v", err)
		}
	}
	if created != member.MaxMembers-1 || full != len(errs)-created || count() != member.MaxMembers {
		t.Errorf("%d created, %d full, %d members, want %d members", created, full, count(), member.MaxMembers)
	}

	// Removing all six at once leaves one.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	errs = make([]error, len(list))
	for i, m := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = repo.Delete(ctx, m.ID)
		}()
	}
	wg.Wait()
	removed, last := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			removed++
		case errors.Is(err, member.ErrLastMember):
			last++
		default:
			t.Errorf("Delete = %v", err)
		}
	}
	if removed != len(list)-1 || last != 1 || count() != 1 {
		t.Errorf("%d removed, %d refused as the last, %d members, want 1", removed, last, count())
	}
}
