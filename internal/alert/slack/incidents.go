package slack

import (
	"sort"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

// SendIncident implements alert.ThreadProvider.
// In token mode it posts rich blocks and threads updates.
// In webhook mode it falls back to SendMessage.
func (s *Slack) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(formatIncidentText(inc, action))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(inc, action, nil)
	}
	return s.SendMessage(formatIncidentText(inc, action))
}

// SendIncidentWithInsight implements alert.InsightThreadProvider: the same as
// SendIncident, with the diagnosis rendered as its own block.
func (s *Slack) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(formatIncidentText(inc, action))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(inc, action, ins)
	}
	return s.SendMessage(formatIncidentText(inc, action))
}

func (s *Slack) sendIncidentWithToken(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	key := string(inc.Key)

	post := s.postBlocks
	if s.postBlocksFn != nil {
		post = s.postBlocksFn
	}

	switch action {
	case model.ActionCreate:
		blocks := buildIncidentBlocksWithInsight(inc, s.appCfg, ins)
		ts, err := post(blocks, "")
		if err != nil {
			return err
		}
		s.saveThread(key, ts)
		return nil

	case model.ActionUpdate:
		threadTS := s.loadThread(key)
		blocks := buildIncidentUpdateBlocksWithInsight(inc, ins)
		_, err := post(blocks, threadTS)
		return err

	case model.ActionResolved:
		threadTS := s.popThread(key)
		blocks := buildIncidentResolvedBlocks(inc)
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
		util.ProviderContext(s.Name()),
		s.channel,
		opts...,
	)
	return ts, wrapSlackRateLimit(err)
}

// SnapshotThreads implements alert.ThreadStateProvider.
func (s *Slack) SnapshotThreads() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.threadMap) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.threadMap))
	for key, ts := range s.threadMap {
		out[key] = ts
	}
	return out
}

// RestoreThreads implements alert.ThreadStateProvider. Restored keys are
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
