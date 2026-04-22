package afcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RenseiAI/agentfactory-tui/afclient"
)

// newAgentReconnectCmd constructs the `agent reconnect <session-id>` subcommand.
// It resumes an orphaned agent session and reports the current status plus the
// number of missed activity events, or emits the raw response in --json mode.
func newAgentReconnectCmd(ds func() afclient.DataSource) *cobra.Command {
	var (
		cursor      string
		lastEventID string
		jsonMode    bool
	)

	cmd := &cobra.Command{
		Use:          "reconnect <session-id>",
		Short:        "Reconnect to an agent session",
		Long:         "Reconnect to an agent session by ID and report the current session status plus any missed activity events.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return errors.New("session id must not be empty")
			}

			req := afclient.ReconnectSessionRequest{}
			if cmd.Flags().Changed("cursor") {
				req.Cursor = &cursor
			}
			if cmd.Flags().Changed("last-event-id") {
				req.LastEventID = &lastEventID
			}

			resp, err := ds().ReconnectSession(id, req)
			if err != nil {
				return fmt.Errorf("reconnect session: %w", err)
			}

			out := cmd.OutOrStdout()
			if jsonMode {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(resp); err != nil {
					return fmt.Errorf("encode reconnect response: %w", err)
				}
				return nil
			}

			verb := "reconnected to"
			if !resp.Reconnected {
				verb = "reconnect declined for"
			}
			message := fmt.Sprintf("%s %s (status: %s, missed: %d %s)",
				verb,
				resp.SessionID,
				resp.SessionStatus,
				resp.MissedEvents,
				pluralizeEvents(resp.MissedEvents),
			)
			if !resp.Reconnected {
				return errors.New(message)
			}

			_, _ = fmt.Fprintln(out, message)
			return nil
		},
	}

	cmd.Flags().StringVar(&cursor, "cursor", "", "Resume from a coordinator activity cursor")
	cmd.Flags().StringVar(&lastEventID, "last-event-id", "", "Resume after the specified activity event ID")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "Output raw JSON (indented)")

	return cmd
}

func pluralizeEvents(n int) string {
	if n == 1 {
		return "event"
	}
	return "events"
}
