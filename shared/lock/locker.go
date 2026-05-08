package lock

import "context"

// Locker is a distributed mutual-exclusion lock keyed by an arbitrary string ID.
type Locker interface {
	TryLock(ctx context.Context, id string) (bool, error)
	Unlock(ctx context.Context, id string) error
}
