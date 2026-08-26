package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/diagridio/dev-dashboard/pkg/statestore"
)

// service is the store-backed Service.
type service struct {
	store  statestore.Store
	reader statestore.RecordReader
}

// New builds a Service over an opened store. A nil store yields ErrNoStore
// from every method — cmd builds a degraded entry that way. A non-nil store
// whose backend does not implement RecordReader yields ErrNotBrowsable.
func New(store statestore.Store, rr statestore.RecordReader) Service {
	return &service{store: store, reader: rr}
}

// ready reports why the service cannot serve, or nil.
func (s *service) ready() error {
	if s.store == nil {
		return ErrNoStore
	}
	if s.reader == nil {
		return ErrNotBrowsable
	}
	return nil
}

// AppIDs returns the sorted distinct key prefixes in the store.
//
// It reads keys only — no values — and is deliberately filter-independent:
// it ignores Search, AppID and IncludeInternal, so selecting an app never
// collapses the dropdown to that one app, and toggling internal keys never
// changes the available prefixes (app and workflow keys share a prefix).
// Unprefixed keys contribute nothing; those records are still listed and
// reachable under "All apps".
func (s *service) AppIDs(ctx context.Context) ([]string, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	keys, _, err := s.store.Keys(ctx, "%", "", 0)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(keys))
	var ids []string
	for _, k := range keys {
		p := classify(k)
		if p.AppID == "" {
			continue
		}
		if _, dup := seen[p.AppID]; dup {
			continue
		}
		seen[p.AppID] = struct{}{}
		ids = append(ids, p.AppID)
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes each key, reporting per-key outcomes so a partial failure
// names exactly which keys survived. There is no second mechanism: unlike a
// workflow instance, a state record has no lifecycle to terminate.
func (s *service) Delete(ctx context.Context, keys []string) []DeleteResult {
	out := make([]DeleteResult, 0, len(keys))
	if err := s.ready(); err != nil {
		for _, k := range keys {
			out = append(out, DeleteResult{Key: k, Error: err.Error()})
		}
		return out
	}
	for _, k := range keys {
		res := DeleteResult{Key: k}
		if err := s.store.Delete(ctx, k); err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
		}
		out = append(out, res)
	}
	return out
}

// unreachable is the Service for a known store whose backend could not be
// opened. Every method fails with a store-specific ErrStoreUnreachable so the
// API can surface an accurate "could not connect…" message. There is no
// sidecar fallback: Dapr's HTTP State API cannot enumerate keys.
type unreachable struct{ name, conn string }

// NewUnreachable builds a Service that always reports ErrStoreUnreachable.
func NewUnreachable(name, conn string) Service { return unreachable{name: name, conn: conn} }

func (u unreachable) err() error {
	return fmt.Errorf("%w %q (%s)", ErrStoreUnreachable, u.name, u.conn)
}

func (u unreachable) List(context.Context, ListQuery) (ListResult, error) {
	return ListResult{}, u.err()
}
func (u unreachable) Record(context.Context, string) (Record, error) { return Record{}, u.err() }
func (u unreachable) AppIDs(context.Context) ([]string, error)       { return nil, u.err() }
func (u unreachable) Delete(_ context.Context, keys []string) []DeleteResult {
	out := make([]DeleteResult, 0, len(keys))
	for _, k := range keys {
		out = append(out, DeleteResult{Key: k, Error: u.err().Error()})
	}
	return out
}

func (s *service) List(ctx context.Context, q ListQuery) (ListResult, error) {
	if err := s.ready(); err != nil {
		return ListResult{}, err
	}
	return ListResult{}, nil
}

func (s *service) Record(ctx context.Context, key string) (Record, error) {
	if err := s.ready(); err != nil {
		return Record{}, err
	}
	return Record{}, ErrNotFound
}
