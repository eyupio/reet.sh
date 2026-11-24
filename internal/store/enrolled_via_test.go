package store

import "testing"

// A machine enrolled before enrolment codes existed was enrolled with a
// personal token: its row gets "token" and it is marked as such.
func TestMachinesFromBeforeCodesCountAsTokenEnrolled(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	// The insert every earlier version made, which names no enrolment.
	if _, err = st.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,source_commit,last_seen,paused,auto_update) VALUES('old',?,'laptop',?,'v1','',1,0,1)`, uid, secretHash("s")); err != nil {
		t.Fatal(err)
	}
	agents, err := st.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || agents[0].EnrolledVia != EnrolledByToken || !agents[0].LegacyToken {
		t.Fatalf("%+v %v", agents, err)
	}
	if _, a, err := st.AuthenticateRemoteAgent("old", "s", ""); err != nil || a.EnrolledVia != EnrolledByToken {
		t.Fatalf("%+v %v", a, err)
	}
}
