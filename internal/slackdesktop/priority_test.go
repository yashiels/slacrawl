package slackdesktop

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/openclaw/slacrawl/internal/slackmcp"
	"github.com/openclaw/slacrawl/internal/store"
	"github.com/stretchr/testify/require"
)

func TestReduxPreservesHigherPriorityMessage(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	defer st.Close()
	now := time.Now().UTC()
	require.NoError(t, st.UpsertWorkspace(ctx, store.Workspace{ID: "T111", Name: "test", UpdatedAt: now}))
	require.NoError(t, st.UpsertChannel(ctx, store.Channel{ID: "C111", WorkspaceID: "T111", Name: "test", UpdatedAt: now}))
	api := store.Message{
		WorkspaceID: "T111", ChannelID: "C111", TS: "1710000000.000001",
		Text: "authoritative <@U111>", NormalizedText: "authoritative", DeletedTS: "1710000001.000001",
		SourceRank: 1, SourceName: "api-user", RawJSON: `{"source":"api"}`, UpdatedAt: now,
	}
	require.NoError(t, st.UpsertMessage(ctx, api, reduxMentions(api.Text)))
	require.NoError(t, ingestReduxStates(ctx, st, []ReduxDecodedState{{
		WorkspaceID: "T111",
		Channels:    []ReduxChannel{{ID: "C111", Name: "test", ContextTeamID: "T111"}},
		Messages: []ReduxMessage{
			{Channel: "C111", TS: api.TS, Type: "message", Text: "stale <@U222>"},
			{Channel: "C111", TS: "1710000002.000001", Type: "message", Text: "desktoponly"},
		},
	}}, now.Add(time.Hour), ingestFilter{}))
	var text, normalized, deleted, source, raw string
	var rank int
	require.NoError(t, st.DB().QueryRowContext(ctx, `select text, normalized_text, deleted_ts, source_rank, source_name, raw_json from messages where channel_id = ? and ts = ?`, api.ChannelID, api.TS).Scan(&text, &normalized, &deleted, &rank, &source, &raw))
	require.Equal(t, api.Text, text)
	require.Equal(t, api.NormalizedText, normalized)
	require.Equal(t, api.DeletedTS, deleted)
	require.Equal(t, api.SourceRank, rank)
	require.Equal(t, api.SourceName, source)
	require.JSONEq(t, api.RawJSON, raw)
	mentions, err := st.Mentions(ctx, "T111", "U222", 10)
	require.NoError(t, err)
	require.Empty(t, mentions)
	rows, err := st.SearchMessages(ctx, store.SearchOptions{Query: "stale", Mode: store.SearchModeRawFTS, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = st.SearchMessages(ctx, store.SearchOptions{Query: "desktoponly", Mode: store.SearchModeRawFTS, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	api.Text = "updated by API"
	api.NormalizedText = api.Text
	require.NoError(t, st.UpsertMessage(ctx, api, nil))
	require.NoError(t, st.DB().QueryRowContext(ctx, `select text from messages where channel_id = ? and ts = ?`, api.ChannelID, api.TS).Scan(&text))
	require.Equal(t, api.Text, text)
}

func TestMCPReplacesDesktopWithoutDowngrade(t *testing.T) {
	for _, api := range []struct {
		source string
		rank   int
	}{
		{"api-user", 1},
		{"api-bot", 2},
	} {
		t.Run(api.source, func(t *testing.T) {
			ctx := context.Background()
			st := admissionStore(t)
			now := time.Now().UTC()
			message := store.Message{
				WorkspaceID: "T1", ChannelID: "C1", TS: "1793168825.038509",
				Text: "cached", NormalizedText: "cached", SourceName: indexedDBSourceName,
				SourceRank: 3, RawJSON: "{}", UpdatedAt: now,
			}
			require.NoError(t, st.UpsertMessage(ctx, message, nil))
			message.SourceName, message.SourceRank = slackmcp.SourceName, slackmcp.SourceRank
			message.Text, message.NormalizedText = "MCP parent", "MCP parent"
			message.ReplyCount, message.LatestReply = 11, "1793168925.038509"
			written, err := st.UpsertMessageByPriority(ctx, message, nil)
			require.NoError(t, err)
			require.True(t, written)
			want := []map[string]any{{"text": "MCP parent", "reply_count": int64(11), "latest_reply": "1793168925.038509", "source_name": "mcp"}}
			query := "select text,reply_count,latest_reply,source_name from messages where ts='1793168825.038509'"
			rows, err := st.QueryReadOnly(ctx, query)
			require.NoError(t, err)
			require.Equal(t, want, rows)
			state := retentionReduxState("T1", "C1", []ReduxMessage{{TS: message.TS, Text: "stale cache"}})
			require.NoError(t, ingestReduxStates(ctx, st, []ReduxDecodedState{state}, now.Add(time.Hour), ingestFilter{}))
			rows, err = st.QueryReadOnly(ctx, query)
			require.NoError(t, err)
			require.Equal(t, want, rows)
			apiMessage := message
			apiMessage.SourceName, apiMessage.SourceRank = api.source, api.rank
			apiMessage.Text, apiMessage.NormalizedText = "API parent", "API parent"
			apiMessage.ReplyCount, apiMessage.LatestReply = 12, "1793169025.038509"
			written, err = st.UpsertMessageByPriority(ctx, apiMessage, nil)
			require.NoError(t, err)
			require.True(t, written)
			written, err = st.UpsertMessageByPriority(ctx, message, nil)
			require.NoError(t, err)
			require.False(t, written)
			rows, err = st.QueryReadOnly(ctx, query)
			require.NoError(t, err)
			require.Equal(t, []map[string]any{{"text": "API parent", "reply_count": int64(12), "latest_reply": "1793169025.038509", "source_name": api.source}}, rows)
		})
	}
}
