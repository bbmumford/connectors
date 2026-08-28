package connectors

import (
	"context"
	"errors"
	"sort"
	"testing"
)

var ctx = context.Background()

// fakeConnector is a test DirectoryConnector serving a fixed directory.
type fakeConnector struct {
	provider string
	groups   []*DirectoryGroup
	members  map[string][]*DirectoryUser // groupID → members
	users    []*DirectoryUser
	fetchErr error
}

func (f *fakeConnector) Provider() string { return f.provider }
func (f *fakeConnector) FetchGroups(context.Context) ([]*DirectoryGroup, error) {
	return f.groups, f.fetchErr
}
func (f *fakeConnector) FetchGroupMembers(_ context.Context, groupID string) ([]*DirectoryUser, error) {
	return f.members[groupID], nil
}
func (f *fakeConnector) FetchUsers(context.Context) ([]*DirectoryUser, error) { return f.users, nil }

func TestRegistry_RegisterNewAndUnknownFailClosed(t *testing.T) {
	r := NewRegistry()
	r.Register("github", func(c ConnectorConfig) (DirectoryConnector, error) {
		if c.AccessToken == "" {
			return nil, errors.New("token required")
		}
		return &fakeConnector{provider: "github"}, nil
	})
	// registered provider builds
	c, err := r.New("github", ConnectorConfig{AccessToken: "t"})
	if err != nil || c.Provider() != "github" {
		t.Fatalf("new github: %v", err)
	}
	// the factory's own validation surfaces
	if _, err := r.New("github", ConnectorConfig{}); err == nil {
		t.Fatal("factory validation error must surface")
	}
	// an unregistered provider fails closed with a typed error
	_, err = r.New("okta", ConnectorConfig{})
	var unk *ErrUnknownProvider
	if !errors.As(err, &unk) || unk.Provider != "okta" {
		t.Fatalf("unknown provider must be *ErrUnknownProvider; got %v", err)
	}
	if ps := r.Providers(); len(ps) != 1 || ps[0] != "github" {
		t.Fatalf("providers: %v", ps)
	}
}

func TestSync_ReconcilesMembershipAcrossGroupsAndUsers(t *testing.T) {
	conn := &fakeConnector{
		provider: "test",
		groups: []*DirectoryGroup{
			{ExternalID: "g1", Name: "Eng"},
			{ExternalID: "g2", Name: "Ops"},
		},
		members: map[string][]*DirectoryUser{
			"g1": {{ExternalID: "u1", Email: "a@x", Name: "Alice"}, {ExternalID: "u2", Email: "b@x", Name: "Bob"}},
			"g2": {{ExternalID: "u1", Email: "a@x", Name: "Alice"}}, // Alice is in both groups
		},
		// the user list adds Carol (in no group's member list) + a group id for Bob not seen via members
		users: []*DirectoryUser{
			{ExternalID: "u3", Email: "c@x", Name: "Carol"},
			{ExternalID: "u2", Email: "b@x", Name: "Bob", Groups: []string{"g3-extra"}},
		},
	}
	snap, err := Sync(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups) != 2 {
		t.Fatalf("groups: %d", len(snap.Groups))
	}
	byID := map[string]*DirectoryUser{}
	for _, u := range snap.Users {
		byID[u.ExternalID] = u
	}
	if len(byID) != 3 {
		t.Fatalf("expected 3 distinct users (Alice/Bob/Carol); got %d", len(byID))
	}
	// Alice accumulated BOTH groups from membership (deduped, in group order)
	alice := byID["u1"]
	if got := groupsSorted(alice); len(got) != 2 || got[0] != "g1" || got[1] != "g2" {
		t.Fatalf("Alice groups: %v (want g1,g2)", got)
	}
	// Bob has g1 (member of Eng) + g3-extra (from the user list) — union across both sources
	bob := byID["u2"]
	if got := groupsSorted(bob); len(got) != 2 || !contains(got, "g1") || !contains(got, "g3-extra") {
		t.Fatalf("Bob groups: %v (want g1 + g3-extra)", got)
	}
	// Carol is in the snapshot with no groups
	carol := byID["u3"]
	if carol == nil || len(carol.Groups) != 0 {
		t.Fatalf("Carol should appear with no groups; got %+v", carol)
	}
}

func TestSync_FetchErrorFailsWholeSync(t *testing.T) {
	conn := &fakeConnector{provider: "test", fetchErr: errors.New("provider down")}
	if _, err := Sync(ctx, conn); err == nil {
		t.Fatal("a fetch error must fail the whole sync (no partial snapshot)")
	}
}

func groupsSorted(u *DirectoryUser) []string {
	g := append([]string(nil), u.Groups...)
	sort.Strings(g)
	return g
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
