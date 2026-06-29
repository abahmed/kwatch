package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abahmed/kwatch/internal/model"
)

const maxResponseBytes = 64 << 10 // 64KB

type Client struct {
	http     *http.Client
	endpoint string
	redactor *redactor
}

func New(endpoint string) *Client {
	return &Client{
		http:     &http.Client{Timeout: RequestTimeout},
		endpoint: endpoint,
		redactor: newRedactor(),
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func (c *Client) Analyze(ctx context.Context, inc *model.Incident) (string, error) {
	reqBody := chatRequest{Model: modelName, Stream: false, Messages: c.buildMessages(inc)}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm endpoint status %d", resp.StatusCode)
	}
	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm: empty response")
	}
	return out.Choices[0].Message.Content, nil
}
