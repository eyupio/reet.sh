package api

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) worker() {
	for {
		select {
		case <-s.stop:
			return
		case r := <-s.queue:
			s.fetchInto(r)
		}
	}
}

// baseContext is the server's lifetime context. Test servers built as a bare
// literal have none, so they fall back to Background.
func (s *server) baseContext() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *server) fetchInto(r ref.Ref) bool {
	if r.SourceType == "hosted" {
		_, username, _ := strings.Cut(r.Source, "/")
		sk, err := s.st.PublishedHostedSkill(username, r.Slug)
		return err == nil && s.st.SetMeta(r.ID, sk.Slug, sk.Description, 0, 0, nil) == nil
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), 10*time.Second)
	defer cancel()
	m := s.f.Fetch(ctx, r)
	if m.Empty() {
		_ = s.st.SetSetting("meta_last_error", "metadata unavailable")
		if s.debug {
			log.Printf("meta fetch id=%s result=empty", r.ID)
		}
		return false
	}
	if err := s.st.SetMeta(r.ID, m.Name, m.Description, m.Installs, m.InstallsWeek, m.Trend); err != nil {
		_ = s.st.SetSetting("meta_last_error", err.Error())
		if s.debug {
			log.Printf("meta store id=%s error=%v", r.ID, err)
		}
		return false
	}
	_ = s.st.SetSetting("meta_last_ok_at", strconv.FormatInt(time.Now().UnixMilli(), 10))
	_ = s.st.SetSetting("meta_last_error", "")
	if s.debug {
		log.Printf("meta fetch id=%s name=%q installs=%d", r.ID, m.Name, m.Installs)
	}
	return true
}

func (s *server) enqueue(r ref.Ref) {
	select {
	case <-s.stop:
	case s.queue <- r:
	default:
	}
}

func (s *server) topicWorker() {
	s.syncTopics()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.syncTopics()
		}
	}
}

func (s *server) syncTopics() {
	if s.st.Fenced() {
		return
	}
	if v, _ := s.st.GetSetting("topics_enabled"); v == "0" {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), 90*time.Second)
	defer cancel()
	topics, err := s.f.Topics(ctx)
	if err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics fetch error=%v", err)
		}
		return
	}
	converted := make([]store.Topic, len(topics))
	for i, t := range topics {
		converted[i] = store.Topic{Slug: t.Slug, Name: t.Name, Skills: t.Skills}
	}
	if err := s.st.ReplaceTopicsForAll(converted); err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics sync error=%v", err)
		}
		return
	}
	if s.debug {
		log.Printf("topics synced count=%d", len(converted))
	}
}

func (s *server) refreshWorker() {
	delay := time.NewTimer(10 * time.Second)
	defer delay.Stop()
	select {
	case <-s.stop:
		return
	case <-delay.C:
	}
	s.refreshBatch()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.refreshBatch()
		}
	}
}

func (s *server) refreshBatch() {
	if s.st.Fenced() {
		return
	}
	if v, _ := s.st.GetSetting("refresh_enabled"); v == "0" {
		return
	}
	ids, err := s.st.RefreshQueue(50)
	if err != nil {
		if s.debug {
			log.Printf("refresh queue error=%v", err)
		}
		return
	}
	failures := 0
	for _, id := range ids {
		if s.ctx != nil && s.ctx.Err() != nil {
			return
		}
		rf, err := ref.Parse(id)
		if err != nil {
			continue
		}
		if s.fetchInto(rf) {
			failures = 0
		} else {
			_ = s.st.TouchFetched(id)
			failures++
			if failures >= 3 {
				if s.debug {
					log.Printf("refresh backing off after %d failures", failures)
				}
				if !s.wait(time.Hour) {
					return
				}
				failures = 0
			}
		}
		if !s.wait(2 * time.Second) {
			return
		}
	}
}

// wait sleeps for d, or returns false when the server stops.
func (s *server) wait(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.stop:
		return false
	case <-timer.C:
		return true
	}
}
