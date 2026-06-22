package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Msg is one chat turn. Role is "user" or "assistant".
type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Oracle is a thin client over the Anthropic Messages API. It mirrors the call
// shape proven in the site's functions/api/oracle.ts: cached system block,
// max_tokens 800, no sampling params (they 400 on opus-4-8), refusal handling.
type Oracle struct {
	key    string
	model  string
	client *http.Client
}

func NewOracle(key, model string) *Oracle {
	if model == "" {
		model = "claude-haiku-4-5"
	}
	return &Oracle{
		key:    key,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type sysBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type reqBody struct {
	Model     string     `json:"model"`
	MaxTokens int        `json:"max_tokens"`
	System    []sysBlock `json:"system"`
	Messages  []Msg      `json:"messages"`
}

type respBody struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ErrRefused signals the model declined the request.
var ErrRefused = fmt.Errorf("refused")

// Ask sends the conversation under the given system prompt and returns the
// assistant's text. Returns ErrRefused if the model refuses.
func (o *Oracle) Ask(ctx context.Context, system string, msgs []Msg) (string, error) {
	if o.key == "" {
		return "", fmt.Errorf("not configured: no API key")
	}
	payload := reqBody{
		Model:     o.model,
		MaxTokens: 800,
		System: []sysBlock{{
			Type:         "text",
			Text:         system,
			CacheControl: &cacheControl{Type: "ephemeral"},
		}},
		Messages: msgs,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", o.key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var data respBody
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if data.Error != nil {
			return "", fmt.Errorf("upstream %d: %s", resp.StatusCode, data.Error.Message)
		}
		return "", fmt.Errorf("upstream %d", resp.StatusCode)
	}
	if data.StopReason == "refusal" {
		return "", ErrRefused
	}
	var out string
	for _, b := range data.Content {
		if b.Type == "text" {
			out += b.Text
		}
	}
	if out == "" {
		return "", fmt.Errorf("empty reply")
	}
	return out, nil
}
