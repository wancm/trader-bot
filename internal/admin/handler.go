package admin

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/wancm/trader-bot/internal/shared"
)

type flagRequest struct {
	Flag bool `json:"flag"`
}

type flagResponse struct {
	Feature string `json:"feature"`
	Enabled bool   `json:"enabled"`
}

// NewMux returns the admin HTTP mux with feature-flag endpoints and swagger UI.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/features", cors(serveFlags))
	mux.HandleFunc("POST /admin/features/tick-listening", cors(featureHandler("tick-listening", &shared.TickListening)))
	mux.HandleFunc("POST /admin/features/make-orders", cors(featureHandler("make-orders", &shared.MakeOrders)))
	mux.HandleFunc("OPTIONS /admin/features/tick-listening", cors(serveFlags))
	mux.HandleFunc("OPTIONS /admin/features/make-orders", cors(serveFlags))
	mux.HandleFunc("GET /swagger.json", serveSpec)
	mux.HandleFunc("GET /swagger/", serveUI)
	return mux
}

func serveFlags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"tick-listening": shared.TickListening.Load(),
		"make-orders":    shared.MakeOrders.Load(),
	})
}

func featureHandler(name string, flag *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req flagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		flag.Store(req.Flag)
		writeJSON(w, http.StatusOK, flagResponse{Feature: name, Enabled: req.Flag})
	}
}

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// ---- Swagger ---------------------------------------------------------------

func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(swaggerSpec)) //nolint:errcheck
}

func serveUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUI)) //nolint:errcheck
}

const swaggerSpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "trader-bot Admin API",
    "description": "Runtime feature flag control for trader-bot.",
    "version": "1.0.0"
  },
  "servers": [{ "url": "http://127.0.0.1:5001" }],
  "tags": [{ "name": "Feature Flags", "description": "Toggle runtime behaviour without restarting the service." }],
  "paths": {
    "/admin/features/tick-listening": {
      "post": {
        "tags": ["Feature Flags"],
        "summary": "Toggle tick listening",
        "description": "**true (default):** MT5 ticks are processed, forwarded to the logger and broker-manager.\n\n**false:** OnTick is a no-op — no price updates, no signals, no logger/broker traffic.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/FlagRequest" },
              "example": { "flag": false }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Flag updated successfully.",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/FlagResponse" },
                "example": { "feature": "tick-listening", "enabled": false }
              }
            }
          },
          "400": { "description": "Invalid JSON body." }
        }
      }
    },
    "/admin/features/make-orders": {
      "post": {
        "tags": ["Feature Flags"],
        "summary": "Toggle order making",
        "description": "**true (default):** AI is called on every signal and orders are sent to the broker.\n\n**false:** Ticks and signals still flow, but the AI call and PlaceOrder are skipped.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/FlagRequest" },
              "example": { "flag": false }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Flag updated successfully.",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/FlagResponse" },
                "example": { "feature": "make-orders", "enabled": false }
              }
            }
          },
          "400": { "description": "Invalid JSON body." }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "FlagRequest": {
        "type": "object",
        "required": ["flag"],
        "properties": {
          "flag": { "type": "boolean", "description": "true to enable, false to disable." }
        }
      },
      "FlagResponse": {
        "type": "object",
        "properties": {
          "feature": { "type": "string", "example": "tick-listening" },
          "enabled": { "type": "boolean", "example": false }
        }
      }
    }
  }
}`

const swaggerUI = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>trader-bot Admin API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; }</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: '/swagger.json',
  dom_id: '#swagger-ui',
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
  layout: 'BaseLayout',
  tryItOutEnabled: true,
  supportedSubmitMethods: ['post'],
});
</script>
</body>
</html>`
