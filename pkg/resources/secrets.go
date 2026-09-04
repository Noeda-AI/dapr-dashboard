package resources

import (
	"context"
	"sort"

	"github.com/diagridio/dev-dashboard/pkg/secrets"
)

// SecretRefStatus is one component metadata field that draws its value from a
// secret reference, with the outcome of resolving it. It never carries the
// value itself — see the reveal endpoint in pkg/server.
type SecretRefStatus struct {
	Field  string `json:"field"`
	Kind   string `json:"kind"`
	Store  string `json:"store,omitempty"`
	Name   string `json:"name,omitempty"`
	Key    string `json:"key,omitempty"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// SecretStoreInfo describes a local secret-store component for its detail pane.
// Keys are names only.
type SecretStoreInfo struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	File       string   `json:"file,omitempty"`
	Prefix     string   `json:"prefix,omitempty"`
	Keys       []string `json:"keys,omitempty"`
	KeysCapped bool     `json:"keysCapped,omitempty"`
	InitErr    string   `json:"initErr,omitempty"`
	UsedBy     []string `json:"usedBy,omitempty"`
}

// secretRefsFor resolves every reference declared in doc. Returns nil when the
// document declares none or no resolver is configured.
func (s *service) secretRefsFor(ctx context.Context, doc []byte) []SecretRefStatus {
	if s.secrets == nil {
		return nil
	}
	storeName, refs := secrets.ParseRefs(doc)
	if len(refs) == 0 {
		return nil
	}
	out := make([]SecretRefStatus, 0, len(refs))
	for field, ref := range refs {
		res := s.secrets.Resolve(ctx, storeName, ref)
		status, detail := res.Status, res.Detail
		if status == secrets.StatusStoreNotFound {
			if p, ok := s.containerStorePath(storeName); ok {
				status = secrets.StatusStoreUnreadable
				detail = "declared inside container " + p +
					"; its secrets are not readable from this host"
			}
		}
		out = append(out, SecretRefStatus{
			Field: field, Kind: ref.Kind, Store: storeName,
			Name: ref.Name, Key: ref.Key,
			Status: string(status), Detail: detail,
		})
	}
	sortByField(out)
	return out
}

// containerStorePath returns the display path of an extras-provided secret
// store with the given name. Extras carry a "<container>:<in-container-path>"
// display path, so the host filesystem has no copy of the store's data.
func (s *service) containerStorePath(storeName string) (string, bool) {
	for _, r := range s.extraByKind(KindComponent) {
		if r.Name == storeName && secrets.IsSecretStoreType(r.Type) {
			return r.Path, true
		}
	}
	return "", false
}

// secretStoreInfoFor builds the detail-pane payload for a secret-store
// component, including which components reference it.
func (s *service) secretStoreInfoFor(ctx context.Context, r Resource) *SecretStoreInfo {
	if s.secrets == nil || !secrets.IsSecretStoreType(r.Type) {
		return nil
	}
	var st secrets.Store
	for _, candidate := range s.secrets.Stores(ctx) {
		if candidate.Name == r.Name {
			st = candidate
			break
		}
	}
	if st.Name == "" {
		return nil
	}
	info := &SecretStoreInfo{
		Name: st.Name, Type: st.Type, File: st.File,
		Prefix: st.Properties["prefix"], InitErr: st.InitErr,
	}
	info.Keys, info.KeysCapped, _ = s.secrets.KeyNames(ctx, r.Name)
	info.UsedBy = s.usedBy(ctx, r.Name)
	return info
}

// usedBy returns the names of components whose auth.secretStore is storeName.
func (s *service) usedBy(ctx context.Context, storeName string) []string {
	scanned, err := s.scan(KindComponent)
	if err != nil {
		return nil
	}
	all := append(scanned, s.extraByKind(KindComponent)...)
	var out []string
	seen := map[string]bool{}
	for _, r := range all {
		declared, refs := secrets.ParseRefs(r.doc)
		if declared == storeName && len(refs) > 0 && !seen[r.Name] {
			seen[r.Name] = true
			out = append(out, r.Name)
		}
	}
	sortStrings(out)
	return out
}

func sortByField(in []SecretRefStatus) {
	sort.Slice(in, func(i, j int) bool { return in[i].Field < in[j].Field })
}

func sortStrings(in []string) { sort.Strings(in) }
