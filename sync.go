package connectors

import "context"

// Snapshot is a reconciled point-in-time view of a directory: every group, and every user with the set
// of group ids it belongs to resolved from group membership. It is the neutral result a consumer maps
// onto its own identity graph (e.g. census entities + edges).
type Snapshot struct {
	Groups []*DirectoryGroup
	Users  []*DirectoryUser
}

// Sync walks a connector into a Snapshot: it fetches every group, then each group's members (attributing
// the group id to each member), then the full user list — merging so a user discovered via membership and
// via the user list appears once with the union of its groups. This is the source's directory-sync
// reconciliation, provider-agnostic: it runs over any DirectoryConnector. A fetch error fails the whole
// sync (no partial snapshot is returned, so a caller never persists a half-synced directory).
func Sync(ctx context.Context, c DirectoryConnector) (*Snapshot, error) {
	groups, err := c.FetchGroups(ctx)
	if err != nil {
		return nil, err
	}
	// users keyed by ExternalID (fall back to Email when a provider omits an id), each carrying a set of
	// group ids so a member seen in several groups accumulates them without duplicates.
	byKey := map[string]*DirectoryUser{}
	groupsOf := map[string]map[string]bool{}
	touch := func(u *DirectoryUser) *DirectoryUser {
		key := u.ExternalID
		if key == "" {
			key = u.Email
		}
		existing, ok := byKey[key]
		if !ok {
			cp := *u
			cp.Groups = nil
			byKey[key] = &cp
			groupsOf[key] = map[string]bool{}
			existing = &cp
		}
		return existing
	}
	addGroup := func(u *DirectoryUser, groupID string) {
		key := u.ExternalID
		if key == "" {
			key = u.Email
		}
		if groupID != "" {
			groupsOf[key][groupID] = true
		}
	}

	for _, g := range groups {
		members, err := c.FetchGroupMembers(ctx, g.ExternalID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			touch(m)
			addGroup(m, g.ExternalID)
		}
	}

	users, err := c.FetchUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		touch(u)
		for _, gid := range u.Groups {
			addGroup(u, gid)
		}
	}

	// materialize each user's group set in stable insertion order isn't guaranteed by a map, so sort for
	// determinism against the group list order.
	orderedGroupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		orderedGroupIDs = append(orderedGroupIDs, g.ExternalID)
	}
	out := &Snapshot{Groups: groups}
	for key, u := range byKey {
		set := groupsOf[key]
		var gids []string
		for _, gid := range orderedGroupIDs {
			if set[gid] {
				gids = append(gids, gid)
			}
		}
		// include any group ids that came from the user list but weren't in the group fetch
		for gid := range set {
			if !containsGroup(orderedGroupIDs, gid) {
				gids = append(gids, gid)
			}
		}
		u.Groups = gids
		out.Users = append(out.Users, u)
	}
	return out, nil
}

func containsGroup(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
