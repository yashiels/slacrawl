package slackmcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseThreadMessagesAllowsBlankReplyAuthor(t *testing.T) {
	raw := `{
		"messages": "=== THREAD PARENT MESSAGE ===\nFrom: Ada (U123)\nTime: 2026-05-02 02:17:47 PDT\nMessage TS: 1777713467.400649\nroot\n\n=== THREAD REPLIES (1 total) ===\n\n--- Reply 1 of 1 ---\nFrom: \nTime: 2026-05-02 04:40:56 PDT\nMessage TS: 1777722056.863759\nreply",
		"pagination_info": "There are no more messages in this thread.\n"
	}`

	thread, err := parseThreadMessages(raw, "C123")
	require.NoError(t, err)
	require.Equal(t, "U123", thread.Parent.AuthorID)
	require.Len(t, thread.Replies, 1)
	require.Equal(t, "1777722056.863759", thread.Replies[0].TS)
	require.Empty(t, thread.Replies[0].AuthorID)
	require.Empty(t, thread.Replies[0].AuthorName)
}

func TestParseThreadMessagesAllowsParentOnlyPayload(t *testing.T) {
	raw := `{
		"messages": "=== THREAD PARENT MESSAGE ===\nFrom: Ada (U123)\nTime: 2026-05-05 14:36:24 PDT\nMessage TS: 1778016984.861209\nroot",
		"pagination_info": "There are no more messages in this thread.\n"
	}`

	thread, err := parseThreadMessages(raw, "C123")
	require.NoError(t, err)
	require.Equal(t, "1778016984.861209", thread.Parent.TS)
	require.Empty(t, thread.Replies)
}

func TestSplitMessageBodyAndMetadataThreadPosition(t *testing.T) {
	for _, tt := range []struct {
		name      string
		raw       string
		text      string
		count     int
		latest    string
		reactions []string
	}{
		{name: "header", raw: "\n\nThread: 9 replies (latest: 2026-10-28 09:45:09 CAT)\nAttachment: alert\nbody", text: "Attachment: alert\nbody", count: 9, latest: "2026-10-28 09:45:09 CAT"},
		{name: "header reactions", raw: "Thread: 10 replies (latest: x)\nReactions: eyes (1)\nAttachment: alert\nbody", text: "Attachment: alert\nbody", count: 10, latest: "x", reactions: []string{"eyes (1)"}},
		{name: "middle reactions", raw: "body\nReactions: eyes (1)\nmore body", text: "body\nReactions: eyes (1)\nmore body"},
		{name: "footer", raw: "body\n\nThread: 3 replies (latest: x)\n", text: "body", count: 3, latest: "x"},
		{name: "middle", raw: "body\nThread: 3 replies (latest: x)\nmore body", text: "body\nThread: 3 replies (latest: x)\nmore body"},
		{name: "none", raw: "\nbody\nmore body\n", text: "body\nmore body"},
		{name: "malformed header", raw: "Thread: 3 replies\nbody", text: "Thread: 3 replies\nbody"},
		{name: "inline header", raw: "Thread: 3 replies (latest: x) extra\nbody", text: "Thread: 3 replies (latest: x) extra\nbody"},
		{name: "header then body thread", raw: "Thread: 9 replies (latest: y)\n\nbody\nThread: 3 replies (latest: x)\nmore body", text: "body\nThread: 3 replies (latest: x)\nmore body", count: 9, latest: "y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text, metadata := splitMessageBodyAndMetadata(tt.raw)
			require.Equal(t, tt.text, text)
			require.Equal(t, tt.count, metadata.ReplyCount)
			require.Equal(t, tt.latest, metadata.LatestReply)
			require.Equal(t, tt.reactions, metadata.Reactions)
		})
	}
}
