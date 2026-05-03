// decision/ai_client_test.go
package decision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIClient_ChatCompletion(t *testing.T) {
	// 模拟 DeepSeek 返回
	expectedResp := AIResponse{
		Choices: []AIChoice{
			{
				Message: AIMessage{
					Content: `{"action":"HOLD","quantity":0,"order_type":"MARKET","reason":"testing","confidence":0.9}`,
				},
			},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResp)
	}))
	defer ts.Close()

	client := NewAIClient("test-key", ts.URL, "deepseek-v4-flash")
	content, err := client.ChatCompletion("system", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AI response: %s", content)
}

func TestPostProcessor(t *testing.T) {
	pp := NewPostProcessor(false, 0.5)

	t.Run("low confidence forced HOLD", func(t *testing.T) {
		aiJSON := `{"action":"BUY","quantity":5,"confidence":0.3,"reason":"test"}`
		d, modified, err := pp.Process(aiJSON, 0, 10, 10000, 100)
		if err != nil {
			t.Fatal(err)
		}
		if d.Action != "HOLD" {
			t.Fatalf("expected HOLD, got %s", d.Action)
		}
		if !modified {
			t.Error("expected modified flag")
		}
	})

	t.Run("short sell blocked", func(t *testing.T) {
		aiJSON := `{"action":"SELL","quantity":1,"confidence":0.8,"reason":"test"}`
		d, modified, err := pp.Process(aiJSON, 0, 10, 10000, 100)
		if err != nil {
			t.Fatal(err)
		}
		if d.Action != "HOLD" {
			t.Fatalf("expected HOLD (short not allowed), got %s", d.Action)
		}
		if !modified {
			t.Error("expected modified")
		}
	})

	t.Run("sell quantity cap", func(t *testing.T) {
		aiJSON := `{"action":"SELL","quantity":10,"confidence":0.8,"reason":"test"}`
		d, modified, err := pp.Process(aiJSON, 5, 10, 10000, 100)
		if err != nil {
			t.Fatal(err)
		}
		if d.Action != "SELL" || d.Quantity != 5 {
			t.Fatalf("expected SELL 5, got %s %.0f", d.Action, d.Quantity)
		}
		if !modified {
			t.Error("expected modified")
		}
	})

	t.Run("buy max limit reached", func(t *testing.T) {
		aiJSON := `{"action":"BUY","quantity":1,"confidence":0.9,"reason":"test"}`
		d, modified, err := pp.Process(aiJSON, 10, 10, 10000, 100)
		if err != nil {
			t.Fatal(err)
		}
		if d.Action != "HOLD" {
			t.Fatalf("expected HOLD, got %s", d.Action)
		}
		_ = modified // 可以忽略
	})
}
