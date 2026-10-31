package terminalproto

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const questionID = "question-0123456789abcdef"

func sampleQuestion() *ACPQuestion {
	return &ACPQuestion{Message: "Which option?", Fields: []ACPQuestionField{
		{Key: "question_0", Title: "Probe", Kind: "single", Required: true, Options: []ACPQuestionOption{
			{Value: "Option A", Label: "Option A", Description: "First"}, {Value: "Option B", Label: "Option B"}}},
		{Key: "tags", Kind: "multi", MinItems: 1, MaxItems: 2, Options: []ACPQuestionOption{{Value: "x", Label: "x"}, {Value: "y", Label: "y"}, {Value: "z", Label: "z"}}},
		{Key: "question_0_custom", Title: "Other", Kind: "text", MaxLength: 500},
	}}
}

func TestQuestionBodiesRoundTrip(t *testing.T) {
	for name, body := range map[string]Body{
		"question":         &ACPEvent{Kind: "question", QuestionID: questionID, ToolCallID: "toolu_q", Question: sampleQuestion()},
		"question no tool": &ACPEvent{Kind: "question", QuestionID: questionID, Question: sampleQuestion()},
		"resolved accept":  &ACPEvent{Kind: "question_resolved", QuestionID: questionID, Reason: "accept"},
		"resolved decline": &ACPEvent{Kind: "question_resolved", QuestionID: questionID, Reason: "decline"},
		"cancelled":        &ACPEvent{Kind: "question_cancelled", QuestionID: questionID, Reason: "timeout"},
		"answer accept":    &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"question_0":"Option A","tags":["x"]}`)},
		"answer decline":   &ACPQuestionAnswer{QuestionID: questionID, Action: "decline"},
		"answer cancel":    &ACPQuestionAnswer{QuestionID: questionID, Action: "cancel"},
	} {
		data, err := Encode(acpMessage(body))
		if err != nil {
			t.Fatalf("%s: encode: %v", name, err)
		}
		decoded, err := Decode(data)
		if err != nil || decoded.Body.kind() != body.kind() {
			t.Fatalf("%s: decode: %v %+v", name, err, decoded.Body)
		}
	}
}

func TestQuestionBodiesAreRejected(t *testing.T) {
	allow := true
	manyFields := sampleQuestion()
	for i := 0; i < MaxACPQuestionFields; i++ {
		manyFields.Fields = append(manyFields.Fields, ACPQuestionField{Key: fmt.Sprintf("f%d", i), Kind: "text", MaxLength: 10})
	}
	manyOptions := &ACPQuestion{Message: "m", Fields: []ACPQuestionField{{Key: "k", Kind: "single"}}}
	for i := 0; i <= MaxACPQuestionOptions; i++ {
		manyOptions.Fields[0].Options = append(manyOptions.Fields[0].Options, ACPQuestionOption{Value: fmt.Sprintf("o%d", i), Label: "o"})
	}
	oversized := &ACPQuestion{Message: "m"}
	for i := 0; i < MaxACPQuestionFields; i++ {
		field := ACPQuestionField{Key: fmt.Sprintf("f%d", i), Kind: "single"}
		for j := 0; j < MaxACPQuestionOptions; j++ {
			field.Options = append(field.Options, ACPQuestionOption{Value: fmt.Sprintf("v%d", j), Label: strings.Repeat("l", MaxACPQuestionLabel), Description: strings.Repeat("d", MaxACPQuestionDescription)})
		}
		oversized.Fields = append(oversized.Fields, field)
	}
	mutate := func(f func(q *ACPQuestion)) *ACPQuestion {
		q := sampleQuestion()
		f(q)
		return q
	}
	for name, body := range map[string]Body{
		"question no payload":      &ACPEvent{Kind: "question", QuestionID: questionID},
		"question no id":           &ACPEvent{Kind: "question", Question: sampleQuestion()},
		"question bad id":          &ACPEvent{Kind: "question", QuestionID: "x", Question: sampleQuestion()},
		"question with permission": &ACPEvent{Kind: "question", QuestionID: questionID, PermissionID: "permission-0123456789", Question: sampleQuestion()},
		"question with command":    &ACPEvent{Kind: "question", QuestionID: questionID, Command: "rm -rf /", Question: sampleQuestion()},
		"question with allow":      &ACPEvent{Kind: "question", QuestionID: questionID, Allow: &allow, Question: sampleQuestion()},
		"question with reason":     &ACPEvent{Kind: "question", QuestionID: questionID, Reason: "x", Question: sampleQuestion()},
		"no message":               &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Message = "" })},
		"no fields":                &ACPEvent{Kind: "question", QuestionID: questionID, Question: &ACPQuestion{Message: "m"}},
		"too many fields":          &ACPEvent{Kind: "question", QuestionID: questionID, Question: manyFields},
		"too many options":         &ACPEvent{Kind: "question", QuestionID: questionID, Question: manyOptions},
		"oversized as json":        &ACPEvent{Kind: "question", QuestionID: questionID, Question: oversized},
		"repeated key":             &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[2].Key = "tags" })},
		"unknown kind":             &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[0].Kind = "number" })},
		"text with options":        &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[2].Options = q.Fields[0].Options })},
		"text no limit":            &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[2].MaxLength = 0 })},
		"single no options":        &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[0].Options = nil })},
		"repeated option":          &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[0].Options[1].Value = "Option A" })},
		"empty label":              &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[0].Options[0].Label = "" })},
		"nul in message":           &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Message = "a\x00b" })},
		"multi bad bounds":         &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[1].MaxItems = 9 })},
		"multi min over max":       &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[1].MinItems = 3 })},
		"single with items":        &ACPEvent{Kind: "question", QuestionID: questionID, Question: mutate(func(q *ACPQuestion) { q.Fields[0].MaxItems = 1 })},
		"resolved payload":         &ACPEvent{Kind: "question_resolved", QuestionID: questionID, Reason: "accept", Question: sampleQuestion()},
		"resolved cancel":          &ACPEvent{Kind: "question_resolved", QuestionID: questionID, Reason: "cancel"},
		"resolved no reason":       &ACPEvent{Kind: "question_resolved", QuestionID: questionID},
		"cancelled payload":        &ACPEvent{Kind: "question_cancelled", QuestionID: questionID, Reason: "x", Question: sampleQuestion()},
		"agent with question id":   &ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "x", QuestionID: questionID},
		"permission with question": &ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", Command: "x", Question: sampleQuestion()},
		"answer bad id":            &ACPQuestionAnswer{QuestionID: "x", Action: "decline"},
		"answer bad action":        &ACPQuestionAnswer{QuestionID: questionID, Action: "allow"},
		"accept no answers":        &ACPQuestionAnswer{QuestionID: questionID, Action: "accept"},
		"accept array":             &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`["a"]`)},
		"accept bad json":          &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"a":`)},
		"accept duplicate keys":    &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"a":"1","a":"2"}`)},
		"accept nested duplicate":  &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"a":{"b":1,"b":2}}`)},
		"accept oversized":         &ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"a":"` + strings.Repeat("y", MaxACPQuestionAnswerBytes) + `"}`)},
		"decline with answers":     &ACPQuestionAnswer{QuestionID: questionID, Action: "decline", Answers: json.RawMessage(`{}`)},
		"cancel with answers":      &ACPQuestionAnswer{QuestionID: questionID, Action: "cancel", Answers: json.RawMessage(`{}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if data, err := Encode(acpMessage(body)); err == nil {
				if _, err := Decode(data); err == nil {
					t.Fatal("accepted")
				}
			}
		})
	}
}

func TestQuestionAnswerNeedsASessionAndRefusesDuplicateKeys(t *testing.T) {
	body := &ACPQuestionAnswer{QuestionID: questionID, Action: "decline"}
	bare := Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: body}
	if err := bare.Validate(); err == nil {
		t.Fatal("an answer without a session was accepted")
	}
	good, err := Encode(acpMessage(&ACPQuestionAnswer{QuestionID: questionID, Action: "accept", Answers: json.RawMessage(`{"a":"1"}`)}))
	if err != nil {
		t.Fatal(err)
	}
	dup := strings.Replace(string(good), `{"a":"1"}`, `{"a":"1","a":"2"}`, 1)
	if dup == string(good) {
		t.Fatal("fixture did not change")
	}
	if _, err := Decode([]byte(dup)); err == nil {
		t.Fatal("duplicate answer keys were accepted")
	}
}

func TestQuestionCapabilityRequiresChat(t *testing.T) {
	c := chatCapabilities()
	c.ACPQuestions = true
	if err := c.Validate(); err != nil {
		t.Fatalf("a chat peer may negotiate questions: %v", err)
	}
	c.ACP = false
	if c.Validate() == nil {
		t.Fatal("questions without chat were accepted")
	}
}

func TestQuestionFeatureNegotiation(t *testing.T) {
	if !SupportsACPQuestion("acp-v1, acp-question-v1") || SupportsACPQuestion("acp-v1,acp-approval-v1") || SupportsACPQuestion("") {
		t.Fatal("feature parsing")
	}
}

func TestQuestionInvalidResultCarriesABoundedDetail(t *testing.T) {
	good := Result{Code: QuestionInvalid, Detail: "answer rejected for question_0: that choice was not offered"}
	data, err := Encode(Message{Version: Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1, Body: good})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]Result{
		"too long":               {Code: QuestionInvalid, Detail: strings.Repeat("x", 201)},
		"nul":                    {Code: QuestionInvalid, Detail: "a\x00b"},
		"detail on another code": {Code: Conflict, Detail: "nope"},
	} {
		if bad.valid() {
			t.Fatalf("%s accepted", name)
		}
	}
	if !(Result{Code: QuestionInvalid}).valid() {
		t.Fatal("a bare question_invalid should be valid")
	}
}
