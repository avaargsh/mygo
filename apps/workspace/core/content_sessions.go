package core

import "sort"

// SessionsForProvider returns a deterministic list of existing sessions.
// Like Session, the returned session values belong to the UI owner.
func (m *ContentManager) SessionsForProvider(providerID string) []*ContentSession {
	var sessions []*ContentSession
	for _, s := range m.sessions {
		if s.ProviderID == providerID {
			sessions = append(sessions, s)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })
	return sessions
}
