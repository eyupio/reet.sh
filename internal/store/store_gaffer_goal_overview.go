package store

import "encoding/json"

// GafferGoalsFor lists retained goals owned through their companion conversation.
// Archived/missing worker scopes stay visible for review and history cleanup;
// this metadata read grants no worker or context access. Conversations are capped
// at 100 per account, and the query independently bounds its response.
func (s *Store) GafferGoalsFor(uid int64) ([]GafferGoal, error) {
	rows, err := s.db.Query(`SELECT g.data,c.id,c.session_id FROM gaffer_goals g
 JOIN gaffer_conversations c ON c.id=g.conversation_id AND c.user_id=g.user_id
 WHERE g.user_id=? ORDER BY c.id LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferGoal{}
	for rows.Next() {
		var raw, conversation, session string
		if err := rows.Scan(&raw, &conversation, &session); err != nil {
			return nil, err
		}
		var g GafferGoal
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			return nil, err
		}
		if session == "" || g.ConversationID != conversation || g.SessionID != session {
			continue
		}
		g.OwnerID = uid
		out = append(out, g)
	}
	return out, rows.Err()
}
