package slackmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncSkipsOnlyInaccessibleChannelErrors(t *testing.T) {
	for _, code := range []string{"channel_not_found", "not_in_channel", "missing_scope", "channel_not_found_extra"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string `json:"method"`
					Params struct {
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				switch request.Method {
				case "initialize":
					writeRPCResult(t, w, map[string]any{"protocolVersion": "2025-03-26"})
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/list":
					writeRPCResult(t, w, map[string]any{"tools": []map[string]any{{"name": "slack_search_channels"}, {"name": "slack_search_users"}, {"name": "slack_read_channel"}}})
				case "tools/call":
					channel := request.Params.Arguments["channel_id"].(string)
					if channel == "CBAD" {
						writeRPCResult(t, w, map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "execution_failed: " + code + " " + admissionCanary}}})
						return
					}
					writeToolText(t, w, map[string]any{"messages": fmt.Sprintf("Channel: fixture (%s)\n\n=== Message from User (UONE) at 2026-10-28T00:00:00Z === \nMessage TS: 1793184142.965639\nhello", channel)})
				}
			}))
			defer server.Close()
			t.Setenv("TEST_MCP_TOKEN", "test-token")
			cfg := testMCPConfig(server.URL)
			cfg.ConnectorID = ""
			var logs bytes.Buffer
			st := admissionStore(t)
			summary, err := Sync(context.Background(), st, Options{WorkspaceID: "TLOCAL", Channels: []string{"CBEFORE", "CBAD", "CAFTER"}, Since: "2026-10-24T00:00:00Z", Config: cfg, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
			if code != "channel_not_found" && code != "not_in_channel" {
				require.Error(t, err)
				require.NotContains(t, err.Error(), admissionCanary)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, summary.Channels)
			require.Equal(t, 2, summary.Messages)
			rows, err := st.QueryReadOnly(context.Background(), "select channel_id from messages order by channel_id")
			require.NoError(t, err)
			require.Equal(t, []map[string]any{{"channel_id": "CAFTER"}, {"channel_id": "CBEFORE"}}, rows)
			require.Contains(t, logs.String(), "CBAD")
			require.Contains(t, logs.String(), code)
			require.NotContains(t, logs.String(), admissionCanary)
		})
	}
}
