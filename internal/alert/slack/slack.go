package slack

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"

	slackClient "github.com/slack-go/slack"
	"k8s.io/klog/v2"
)

const conversationLockCount = 64

type Slack struct {
	channel     string
	clusterName string
	clockSource clock.Clock

	// webhook mode
	webhook string
	send    func(string, *slackClient.WebhookMessage) error
	// sendContext is the production transport. send remains a small test seam
	// for webhook behavior.
	sendContext func(context.Context, string, *slackClient.WebhookMessage) error

	// token mode
	token     string
	apiClient *slackClient.Client
	// channelID is the channel ID from the latest post response, guarded
	// by mu. chat.update needs it when channel is a name.
	channelID string

	// conversations maps an incident key to its thread. conversationOrder is
	// insertion order so the map is bounded by evicting the oldest thread
	// rather than refusing to record new ones.
	conversations     map[string]conversationState
	conversationOrder []string
	mu                sync.Mutex
	conversationLocks [conversationLockCount]sync.Mutex

	// maxThreadMapSize bounds the thread and conversation maps. When
	// exceeded, the oldest entries are evicted; their later updates post at
	// top level instead of in a thread.
	maxThreadMapSize int

	// compact mode sends single-line messages instead of rich embeds
	compact bool

	// overridable in tests
	postBlocksFn func(
		blocks *slackClient.Blocks, threadTS string,
	) (string, error)
}

func (s *Slack) conversationLock(key string) *sync.Mutex {
	return &s.conversationLocks[lockIndex(key)]
}

// lockIndex is the stripe of conversationLocks that guards key.
func lockIndex(key string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return hash.Sum32() % conversationLockCount
}

// NewSlack returns new Slack instance
func NewSlack(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Slack {
	httpClient := dependencies.HTTPClient
	compact, _ := config["compact"].(bool)

	// token mode: requires token + channel
	token, hasToken := config["token"].(string)
	channel, hasChannel := config["channel"].(string)
	if hasToken && len(token) > 0 {
		if !hasChannel || len(channel) == 0 {
			klog.InfoS("initializing slack with token but missing channel")
			return nil
		}
		klog.InfoS(
			"initializing slack with token and channel",
			"channel",
			channel,
		)
		return &Slack{
			token:       token,
			channel:     channel,
			compact:     compact,
			clusterName: clusterName,
			clockSource: clock.Require(dependencies.Clock),
			apiClient: slackClient.New(
				token,
				slackClient.OptionHTTPClient(httpClient),
			),
			maxThreadMapSize: 1000,
			conversations:    make(map[string]conversationState),
		}
	}

	// webhook mode: requires webhook
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing slack with empty webhook url and no token")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing slack with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing slack with webhook configured")

	return &Slack{
		webhook:          webhook,
		channel:          channel,
		compact:          compact,
		maxThreadMapSize: 1000,
		clusterName:      clusterName,
		clockSource:      clock.Require(dependencies.Clock),
		conversations:    make(map[string]conversationState),
		sendContext: func(
			ctx context.Context,
			url string,
			msg *slackClient.WebhookMessage,
		) error {
			return slackClient.PostWebhookCustomHTTPContext(
				ctx, url, httpClient, msg,
			)
		},
	}
}

func (s *Slack) saveConversation(key string, state conversationState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conversations == nil {
		s.conversations = make(map[string]conversationState)
	}
	s.dropExpiredLocked()
	if _, exists := s.conversations[key]; !exists {
		s.conversationOrder = append(s.conversationOrder, key)
	}
	s.conversations[key] = state
	for s.maxThreadMapSize > 0 &&
		len(s.conversations) > s.maxThreadMapSize &&
		len(s.conversationOrder) > 0 {
		s.removeLocked(s.evictionCandidateLocked(key))
	}
}

// evictionCandidateLocked picks the conversation to drop when the map is
// full: the oldest one kept only for a possible reopen (ReopenUntil set),
// else the oldest of all. An open incident's thread is worth more than a
// resolved one's. The conversation just saved (keep) is never chosen
// unless it is the only one. The caller holds s.mu.
func (s *Slack) evictionCandidateLocked(keep string) string {
	oldest := ""
	for _, key := range s.conversationOrder {
		if key == keep {
			continue
		}
		if !s.conversations[key].ReopenUntil.IsZero() {
			return key
		}
		if oldest == "" {
			oldest = key
		}
	}
	if oldest == "" {
		return keep
	}
	return oldest
}

// dropExpiredLocked forgets resolved conversations whose reopen window
// has passed. The caller holds s.mu.
func (s *Slack) dropExpiredLocked() {
	now := s.clockSource.Now()
	for key, state := range s.conversations {
		if state.expired(now) {
			s.removeLocked(key)
		}
	}
}

func (s *Slack) deleteConversation(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(key)
}

func (s *Slack) removeLocked(key string) {
	delete(s.conversations, key)
	for i, k := range s.conversationOrder {
		if k == key {
			s.conversationOrder = append(
				s.conversationOrder[:i], s.conversationOrder[i+1:]...,
			)
			break
		}
	}
}

// Name returns name of the provider
func (s *Slack) Name() string {
	return "Slack"
}

// Verify checks credentials via Slack auth.test (token mode) or webhook URL.
func (s *Slack) Verify(ctx context.Context) error {
	if s.apiClient != nil {
		_, err := s.apiClient.AuthTestContext(ctx)
		return err
	}
	if s.webhook == "" {
		return fmt.Errorf("slack: no webhook or token configured")
	}
	return nil
}

// SendMessage sends text using the caller's cancellation context.
func (s *Slack) SendMessage(ctx context.Context, msg string) error {
	return s.sendAPI(ctx, &slackClient.WebhookMessage{
		Text: msg,
	})
}
