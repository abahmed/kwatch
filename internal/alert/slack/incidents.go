package slack

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

// SendIncident implements delivery.ThreadProvider.
// In token mode it posts rich blocks and threads updates.
// In webhook mode it falls back to SendMessage.
func (s *Slack) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(ctx, formatIncidentText(
			inc, action, s.clockSource,
		))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(ctx, inc, action, nil)
	}
	return s.SendMessage(ctx, formatIncidentText(
		inc, action, s.clockSource,
	))
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider. It is the
// same as
// SendIncident, with the diagnosis rendered as its own block.
func (s *Slack) SendIncidentWithInsight(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(ctx, formatIncidentText(
			inc, action, s.clockSource,
		))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(ctx, inc, action, ins)
	}
	return s.SendMessage(ctx, formatIncidentText(
		inc, action, s.clockSource,
	))
}

func (s *Slack) sendIncidentWithToken(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	key := string(inc.Key)

	post := func(
		blocks *slackClient.Blocks,
		threadTS string,
	) (string, error) {
		return s.postBlocks(ctx, blocks, threadTS)
	}
	if s.postBlocksFn != nil {
		post = func(
			blocks *slackClient.Blocks,
			threadTS string,
		) (string, error) {
			return s.postBlocksFn(blocks, threadTS)
		}
	}

	switch action {
	case model.ActionCreate:
		blocks := buildIncidentBlocksWithInsight(
			inc, s.clusterName, ins, s.clockSource,
		)
		ts, err := post(blocks, "")
		if err != nil {
			return err
		}
		s.saveThread(key, ts)
		return nil

	case model.ActionUpdate:
		threadTS := s.loadThread(key)
		blocks := buildIncidentUpdateBlocksWithInsight(
			inc, ins, s.clockSource,
		)
		_, err := post(blocks, threadTS)
		return err

	case model.ActionResolved:
		threadTS := s.popThread(key)
		blocks := buildIncidentResolvedBlocks(inc, s.clockSource)
		_, err := post(blocks, threadTS)
		return err
	}

	return nil
}

func (s *Slack) saveThread(key, ts string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threadMap == nil {
		s.threadMap = make(map[string]string)
		s.threadOrder = nil
	}
	if _, exists := s.threadMap[key]; !exists {
		s.threadOrder = append(s.threadOrder, key)
	}
	s.threadMap[key] = ts
	// Refusing to record new threads at capacity meant the newest incidents --
	// the ones still being worked -- lost their thread while long-finished
	// ones kept theirs. Evict oldest-first instead.
	for s.maxThreadMapSize > 0 && len(s.threadMap) > s.maxThreadMapSize &&
		len(s.threadOrder) > 0 {
		oldest := s.threadOrder[0]
		s.threadOrder = s.threadOrder[1:]
		delete(s.threadMap, oldest)
	}
}

// forgetThread drops a key from both the map and the eviction order.
func (s *Slack) forgetThread(key string) {
	delete(s.threadMap, key)
	for i, k := range s.threadOrder {
		if k == key {
			s.threadOrder = append(s.threadOrder[:i], s.threadOrder[i+1:]...)
			break
		}
	}
}

func (s *Slack) loadThread(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.threadMap[key]
	if !ok {
		return ""
	}
	return ts
}

func (s *Slack) popThread(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := s.threadMap[key]
	s.forgetThread(key)
	return ts
}

func (s *Slack) postBlocks(
	ctx context.Context,
	blocks *slackClient.Blocks,
	threadTS string,
) (string, error) {
	opts := []slackClient.MsgOption{
		slackClient.MsgOptionBlocks(blocks.BlockSet...),
		slackClient.MsgOptionAsUser(true),
	}
	if threadTS != "" {
		opts = append(opts, slackClient.MsgOptionTS(threadTS))
	}
	_, ts, err := s.apiClient.PostMessageContext(
		ctx,
		s.channel,
		opts...,
	)
	return ts, wrapSlackRateLimit(err)
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (s *Slack) SnapshotThreads() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.threadMap) == 0 && len(s.conversations) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.threadMap)+len(s.conversations))
	for key, ts := range s.threadMap {
		out[key] = ts
	}
	for key, state := range s.conversations {
		encoded, err := json.Marshal(state)
		if err == nil {
			out[key] = string(encoded)
		}
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider. Restored keys are
// appended to the eviction order in a stable sequence so the bound still
// applies, and existing live threads always win over saved ones.
func (s *Slack) RestoreThreads(saved map[string]string) {
	if len(saved) == 0 {
		return
	}
	keys := make([]string, 0, len(saved))
	for key := range saved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if ts := saved[key]; ts != "" {
			var state conversationState
			if json.Unmarshal([]byte(ts), &state) == nil &&
				state.ThreadTS != "" {
				s.saveConversation(key, state)
				s.adoptThread(key, state.ThreadTS)
				continue
			}
			s.adoptThread(key, ts)
		}
	}
}

// adoptThread records a saved thread id unless this run already posted one
// for that incident: a live thread is always the better target.
func (s *Slack) adoptThread(key, ts string) {
	s.mu.Lock()
	_, exists := s.threadMap[key]
	s.mu.Unlock()
	if exists {
		return
	}
	s.saveThread(key, ts)
}
