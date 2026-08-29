package slack

import (
	"context"

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
	}
	if s.maxThreadMapSize <= 0 || len(s.threadMap) < s.maxThreadMapSize {
		s.threadMap[key] = ts
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
	ts, _ := s.threadMap[key]
	delete(s.threadMap, key)
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
		context.Background(),
		s.channel,
		opts...,
	)
	return ts, wrapSlackRateLimit(err)
}
