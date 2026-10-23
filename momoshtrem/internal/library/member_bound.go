package library

import "errors"

// errUnbound means a member's repository was used before ForMember.
var errUnbound = errors.New("the repository isn't bound to a household member (ForMember)")

// boundMember is the member a repository reads and writes for (ForMember:
// TraktAuthRepository, TraktSyncStore, TasteRepository,
// TasteProfileRepository); 0 until it's bound.
type boundMember int64

// id is the member, or errUnbound.
func (m boundMember) id() (int64, error) {
	if m <= 0 {
		return 0, errUnbound
	}
	return int64(m), nil
}
