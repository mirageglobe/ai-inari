// chat.go owns the Ollama chat and pull calls: the blocking Chat, the streaming
// ChatStream, and PullModel. it does NOT own the REST client core, listing, or
// model load/unload/caps (client.go).

package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// Chat sends a single blocking request and returns the full reply.
func (c *Client) Chat(model string, messages []provider.Message) (string, error) {
	if c.verbose {
		log.Printf("[inarid→ollama] chat model=%s msgs=%d", model, len(messages))
	}
	req := provider.ChatRequest{Model: model, Messages: messages, Stream: false}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", ollamaError(resp)
	}
	defer resp.Body.Close()
	var result provider.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if c.verbose {
		log.Printf("[ollama→inarid] chat ok (%d chars)", len(result.Message.Content))
	}
	return result.Message.Content, nil
}

// PullModel downloads model via Ollama's streaming pull endpoint, forwarding each
// status update to out until "success" or the stream ends. mirrors ChatStream's
// scan-and-forward shape since /api/pull streams newline-delimited JSON the same way.
func (c *Client) PullModel(model string, out chan<- provider.PullProgress) error {
	body, _ := json.Marshal(map[string]any{"model": model, "stream": true})
	resp, err := c.http.Post(c.baseURL+"/api/pull", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return ollamaError(resp)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var p provider.PullProgress
		if err := json.Unmarshal(scanner.Bytes(), &p); err != nil {
			continue
		}
		out <- p
		if p.Status == "success" {
			break
		}
	}
	return scanner.Err()
}

// ChatStream sends a chat request and yields response chunks via a channel.
func (c *Client) ChatStream(req provider.ChatRequest, out chan<- provider.ChatResponse) error {
	if c.verbose {
		log.Printf("[inarid→ollama] chat.stream model=%s msgs=%d", req.Model, len(req.Messages))
	}
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return ollamaError(resp)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk provider.ChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		out <- chunk
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}
