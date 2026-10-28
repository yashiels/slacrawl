package slackmcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncBoundedHistoryPagesByLatest(t *testing.T) {
	for _, ending := range []string{"complete", "repeated", "empty", "limit"} {
		t.Run(ending, func(t *testing.T) {
			calls := 0
			gateway := admissionGateway(t, false, func(name string, args map[string]any) map[string]any {
				require.Equal(t, "slack_read_channel", name)
				calls++
				ts, cursor := "1793184142.965639", "same"
				if args["latest"] == "1793184142.965639" {
					ts = "1793051104.962969"
				}
				if args["latest"] == "1793051104.962969" {
					switch ending {
					case "complete":
						ts, cursor = "1792927322.943269", ""
					case "repeated":
						ts = "1793051104.962969"
					case "empty":
						return map[string]any{"messages": "Channel: one (CONE)\nNo messages found.", "pagination_info": "cursor `same`"}
					}
				}
				if args["oldest"] != nil && args["cursor"] != nil {
					ts = "1793184142.965639"
				}
				pagination := ""
				if cursor != "" {
					pagination = "cursor `" + cursor + "`"
				}
				return map[string]any{"messages": fmt.Sprintf("Channel: one (CONE)\n\n=== Message from User (UONE) at 2026-10-28T00:00:00Z === \nMessage TS: %s\nhello", ts), "pagination_info": pagination}
			})
			defer gateway.Close()
			t.Setenv("TEST_MCP_TOKEN", "test-token")
			cfg := testMCPConfig(gateway.URL)
			cfg.ConnectorID = ""
			if ending == "limit" {
				cfg.MaxPages = 1
			}
			st := admissionStore(t)
			summary, err := Sync(context.Background(), st, Options{WorkspaceID: "TLOCAL", Channels: []string{"CONE"}, Since: "2026-10-24T00:00:00Z", Config: cfg})
			if ending == "limit" {
				require.ErrorContains(t, err, "max_pages=1")
				return
			}
			require.NoError(t, err)
			want := 2
			if ending == "complete" {
				want = 3
			}
			require.Equal(t, want, summary.Messages)
			require.Equal(t, 3, calls)
			rows, err := st.QueryReadOnly(context.Background(), "select ts from messages order by ts")
			require.NoError(t, err)
			require.Len(t, rows, want)
			require.Equal(t, "1793184142.965639", rows[want-1]["ts"])
			require.Equal(t, "1793051104.962969", rows[want-2]["ts"])
		})
	}
}
