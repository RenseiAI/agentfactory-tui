package afcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RenseiAI/agentfactory-tui/afclient"
)

type agentReconnectJSON struct {
	Reconnected   bool                   `json:"reconnected"`
	SessionID     string                 `json:"sessionId"`
	SessionStatus afclient.SessionStatus `json:"sessionStatus"`
	MissedEvents  int                    `json:"missedEvents"`
}

// newAgentReconnectCmd constructs the `agent reconnect <session-id>` subcommand.
// It optionally forwards cursor / last-event-id resume hints and renders either
// a one-line confirmation or indented JSON when --json is set.
func newAgentReconnectCmd(ds func() afclient.DataSource) *cobra.Command {
	var (
		jsonMode    bool
		cursor      string
		lastEventID string
	)

	cmd := &cobra.Command{
		Use:          "reconnect <session-id>",
		Short:        "Reconnect to an agent session",
		Long:         "Reconnect to an orphaned AgentFactory session by ID and report its current status plus any missed activity count.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if strings.TrimSpace(id) == "" {
				return errors.New("session id must not be empty")
			}

			req := afclient.ReconnectSessionRequest{}
			if cursor != "" {
				req.Cursor = &cursor
			}
			if lastEventID != "" {
				req.LastEventID = &lastEventID
			}

			resp, err := ds().ReconnectSession(id, req)
			if err != nil {
				if errors.Is(err, afclient.ErrNotFound) {
					return fmt.Errorf("reconnect %s: %w", id, err)
				}
				return fmt.Errorf("reconnect %s: %w", id, err)
			}

			out := cmd.OutOrStdout()
			if jsonMode {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(agentReconnectJSON{
					Reconnected:   resp.Reconnected,
					SessionID:     resp.SessionID,
					SessionStatus: resp.SessionStatus,
					MissedEvents:  resp.MissedEvents,
				}); err != nil {
					return fmt.Errorf("encode reconnect response: %w", err)
				}
				return nil
			}

			if !resp.Reconnected {
				return fmt.Errorf("reconnect declined for %s (status: %s, missed: %d %s)",
					resp.SessionID, resp.SessionStatus, resp.MissedEvents, eventNoun(resp.MissedEvents))
			}

			_, _ = fmt.Fprintf(out, "reconnected to %s (status: %s, missed: %d %s)\n",
				resp.SessionID, resp.SessionStatus, resp.MissedEvents, eventNoun(resp.MissedEvents))
			return nil
		},
	}

	cmd.Flags().StringVar(&cursor, "cursor", "", "Resume cursor for activity replay")
	cmd.Flags().StringVar(&lastEventID, "last-event-id", "", "Last seen activity event ID")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "Output raw JSON (indented)")

	return cmd
}

func eventNoun(n int) string {
	if n == 1 {
		return "event"
	}
	return "events"
}
