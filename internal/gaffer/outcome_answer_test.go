package gaffer

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// questionStarter is the rig's starter with a session question on top: the
// question its agent is waiting on, and the answers it was given.
type questionStarter struct {
	*outcomeStarterFake
	mu       sync.Mutex
	question *AgentQuestion
	answered []string
}

func (q *questionStarter) OutcomeSessionQuestion(_ context.Context, _ int64, _ string) (AgentQuestion, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.question == nil {
		return AgentQuestion{}, false
	}
	return *q.question, true
}

func (q *questionStarter) AnswerOutcomeSessionQuestion(_ context.Context, _ int64, _, id string, answers json.RawMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.answered = append(q.answered, id+" "+string(answers))
	q.question = nil
	return nil
}

func (q *questionStarter) got() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.answered...)
}

func dbQuestion() terminalproto.ACPQuestion {
	return terminalproto.ACPQuestion{Message: "Which database should the export read from?", Fields: []terminalproto.ACPQuestionField{{Key: "db", Title: "Database", Kind: "single", Required: true, Options: []terminalproto.ACPQuestionOption{{Value: "replica", Label: "The read replica"}, {Value: "primary", Label: "The primary"}}}}}
}

// heldForQuestion starts an outcome whose first stage's agent is waiting on q.
func heldForQuestion(t *testing.T, r *outcomeRig, automation bool, q *AgentQuestion) (*questionStarter, store.GafferOutcome, store.GafferOutcomeSession) {
	t.Helper()
	asker := &questionStarter{outcomeStarterFake: r.starter, question: q}
	r.s.SetSessionStarter(asker)
	in := pipelineInput(r, false)
	in.Automation = automation
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	g, err := r.st.GafferGoalFor(r.p.UserID, piece.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.st.HoldGafferGoalForWorker(g, workerWaitingNote, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	return asker, mustOutcome(t, r, created.ID), piece
}

// Gaffer answers an agent's multiple-choice question on an outcome whose
// decisions it was given: one of the offered options goes to the agent by the
// person's own path, and the outcome keeps what was asked, what was answered
// and why.
func TestGafferAnswersAnAgentsMultipleChoiceQuestionWhenGivenTheDecisions(t *testing.T) {
	r := outcomeFixture(t)
	var calls atomic.Int32
	decideWith(r, &calls, "answer_question", `{"answers":{"db":"replica"},"why":"The brief says the export must not load the primary."}`)
	asker, created, piece := heldForQuestion(t, r, true, &AgentQuestion{ID: "q-1", Question: dbQuestion(), Alone: true})
	r.s.autoDecisions(context.Background())
	got := decisions(t, r, created.ID, 1)[0]
	if got.Failed || got.Choice != "answer" || got.About != "agent_question" || got.Key != piece.Key || !strings.Contains(got.Why, "must not load the primary") || got.Text != "Asked: Which database should the export read from? Answered: The read replica" {
		t.Fatalf("decision: %+v", got)
	}
	if sent := asker.got(); len(sent) != 1 || sent[0] != `q-1 {"db":"replica"}` {
		t.Fatalf("answers sent: %v", sent)
	}
	// The same question is not put to Gaffer twice, and the pipeline did not move.
	r.s.autoDecisions(context.Background())
	time.Sleep(60 * time.Millisecond)
	if o := mustOutcome(t, r, created.ID); calls.Load() != 1 || len(o.Decisions) != 1 || o.Sessions[0].Stage != 0 || o.Sessions[0].Stages[0].Decided != 1 {
		t.Fatalf("after the answer: calls %d, %+v", calls.Load(), o.Sessions[0])
	}
}

// Gaffer can leave a question to the person, and an option the agent did not
// offer is never sent: either way nothing reaches the agent and the outcome
// says the question is the person's.
func TestAQuestionGafferLeavesOrAnswersBadlyStaysWithThePerson(t *testing.T) {
	for name, c := range map[string]struct{ tool, args, says string }{
		"left":        {"leave_to_user", `{"why":"Nothing in the outcome says which database is safe."}`, "Gaffer left this question to you: Nothing in the outcome says which database is safe."},
		"not offered": {"answer_question", `{"answers":{"db":"production"},"why":"It is the real one."}`, "it is yours"},
		"extra field": {"answer_question", `{"answers":{"db":"replica","drop":"yes"},"why":"x"}`, "it is yours"},
	} {
		r := outcomeFixture(t)
		var calls atomic.Int32
		decideWith(r, &calls, c.tool, c.args)
		asker, created, _ := heldForQuestion(t, r, true, &AgentQuestion{ID: "q-1", Question: dbQuestion(), Alone: true})
		r.s.autoDecisions(context.Background())
		got := decisions(t, r, created.ID, 1)[0]
		if !got.Failed || got.Choice != "" || !strings.Contains(got.Why, c.says) || len(asker.got()) != 0 {
			t.Fatalf("%s: %+v, sent %v", name, got, asker.got())
		}
	}
}

// What is Gaffer's to answer: a multiple-choice question that is the only
// thing the agent waits for, on an outcome whose decisions it was given.
// Never an approval beside it, never words only the person can give.
func TestWhichAgentQuestionsGafferMayAnswer(t *testing.T) {
	typed := terminalproto.ACPQuestion{Message: "What should the file be called?", Fields: []terminalproto.ACPQuestionField{{Key: "name", Kind: "text", Required: true, MaxLength: 80}}}
	mixed := dbQuestion()
	mixed.Fields = append(mixed.Fields, terminalproto.ACPQuestionField{Key: "note", Kind: "text", MaxLength: 200})
	if answerable(typed) || !answerable(dbQuestion()) || !answerable(mixed) {
		t.Fatal("answerable: a required text field is the person's; an optional one is left out")
	}
	for name, c := range map[string]struct {
		question *AgentQuestion
		ok       bool
	}{
		"a choice, alone":          {&AgentQuestion{ID: "q-1", Question: dbQuestion(), Alone: true}, true},
		"an approval waits beside": {&AgentQuestion{ID: "q-1", Question: dbQuestion(), Alone: false}, false},
		"the person's own words":   {&AgentQuestion{ID: "q-1", Question: typed, Alone: true}, false},
		"no question, an approval": {nil, false},
	} {
		r := outcomeFixture(t)
		_, o, piece := heldForQuestion(t, r, true, c.question)
		stop, ok := r.s.pipelineStop(o, piece)
		if ok != c.ok || ok && (stop.about != "agent_question" || strings.Join(stop.choices, ",") != "answer" || stop.sign != "q:q-1") {
			t.Fatalf("%s: %+v %v", name, stop, ok)
		}
	}
	// Kept by the person: nothing is asked of the model and nothing is answered.
	r := outcomeFixture(t)
	var calls atomic.Int32
	decideWith(r, &calls, "answer_question", `{"answers":{"db":"replica"},"why":"x"}`)
	asker, created, _ := heldForQuestion(t, r, false, &AgentQuestion{ID: "q-1", Question: dbQuestion(), Alone: true})
	r.s.autoDecisions(context.Background())
	time.Sleep(60 * time.Millisecond)
	if o := mustOutcome(t, r, created.ID); calls.Load() != 0 || len(o.Decisions) != 0 || len(asker.got()) != 0 {
		t.Fatalf("answered for a person who kept the decisions: %d %+v %v", calls.Load(), o.Decisions, asker.got())
	}
}

func TestAnAnswerIsOnlyWhatTheQuestionOffers(t *testing.T) {
	multi := terminalproto.ACPQuestion{Message: "Which checks?", Fields: []terminalproto.ACPQuestionField{{Key: "checks", Kind: "multi", MinItems: 1, MaxItems: 2, Options: []terminalproto.ACPQuestionOption{{Value: "lint"}, {Value: "unit", Label: "Unit tests"}, {Value: "e2e"}}}}}
	answers, why, chosen, err := questionAnswer(json.RawMessage(`{"answers":{"checks":["unit","lint"]},"why":"Fast and enough."}`), multi)
	if err != nil || string(answers) != `{"checks":["unit","lint"]}` || why != "Fast and enough." || chosen != "Unit tests, lint" {
		t.Fatalf("%s %q %q %v", answers, why, chosen, err)
	}
	for _, raw := range []string{
		`{"answers":{"checks":["unit","lint","e2e"]},"why":"x"}`,
		`{"answers":{"checks":[]},"why":"x"}`,
		`{"answers":{"checks":["unit","unit"]},"why":"x"}`,
		`{"answers":{"checks":["deploy"]},"why":"x"}`,
		`{"answers":{"checks":["unit"]}}`,
		`{"answers":{"checks":["unit"]},"why":"x","also":"y"}`,
	} {
		if _, _, _, err := questionAnswer(json.RawMessage(raw), multi); err == nil {
			t.Fatalf("accepted: %s", raw)
		}
	}
	// The tool's schema is the question's own options.
	schema := string(answerTool(dbQuestion()).Parameters)
	if !strings.Contains(schema, `"enum":["replica","primary"]`) || !strings.Contains(schema, `"required":["db"]`) {
		t.Fatalf("schema: %s", schema)
	}
}
