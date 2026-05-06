// decision/ai_client.go
package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AIRequest is the chat completion request body.
type AIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIChoice is one completion choice.
type AIChoice struct {
	Message AIMessage `json:"message"`
}

// AIMessage is the message inside a choice.
type AIMessage struct {
	Content string `json:"content"`
}

// AIResponse is the full response from DeepSeek.
type AIResponse struct {
	Choices []AIChoice `json:"choices"`
}

// AIDecision is the parsed trading decision from the AI.
type AIDecision struct {
	Action               string  `json:"action"`
	Quantity             float64 `json:"quantity"`
	OrderType            string  `json:"order_type"`
	LimitPrice           float64 `json:"limit_price,omitempty"`
	Reason               string  `json:"reason"`
	Confidence           float64 `json:"confidence"`
	StopLossSuggestion   float64 `json:"stop_loss_suggestion,omitempty"`
	TakeProfitSuggestion float64 `json:"take_profit_suggestion,omitempty"`
}

// AIClient handles communication with the DeepSeek API.
type AIClient struct {
	APIKey          string
	BaseURL         string
	Model           string
	ReasoningEffort string // 新增：控制推理深度，如 "medium", "low"
	HTTPClient      *http.Client
}

// NewAIClient creates a new DeepSeek client.
func NewAIClient(apiKey, baseURL, model string) *AIClient {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	return &AIClient{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ChatCompletion sends a completion request and returns the message content.
func (c *AIClient) ChatCompletion(systemPrompt, userContent string) (string, error) {
	reqBody := map[string]interface{}{
		"model": c.Model,
		"messages": []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
	}

	// Only enable thinking mode when explicitly configured (incurs higher cost).
	// Leave unset for standard flash (non-thinking) mode.
	if c.ReasoningEffort != "" {
		reqBody["reasoning_effort"] = c.ReasoningEffort
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepseek api error %d: %s", resp.StatusCode, string(body))
	}

	var aiResp AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(aiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}

	return aiResp.Choices[0].Message.Content, nil
}
