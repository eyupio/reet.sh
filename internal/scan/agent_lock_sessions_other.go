//go:build !linux && !darwin

package scan

// noSessions is a platform where reet runs no terminal sessions or chats.
type noSessions struct{}

func (noSessions) running() (int, error) { return 0, nil }
func (noSessions) endAll() (int, error)  { return 0, nil }

func localSessionsFor(string, agentConfig) machineSessions { return noSessions{} }
