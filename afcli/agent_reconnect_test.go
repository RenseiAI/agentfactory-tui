package afcli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RenseiAI/agentfactory-tui/afclient"
)

type reconnectStubDataSource struct {
	stubDataSource
	reconnectResp  *afclient.ReconnectSessionResponse
	reconnectErr   error
	reconnectCalls int
	lastID         string
	lastReq        afclient.ReconnectSessionRequest
	failIfInvoked  *testing.T
}

func (r *reconnectStubDataSource) ReconnectSession(id string, req afclient.ReconnectSessionRequest) (*afclient.ReconnectSessionResponse, error) {
	r.reconnectCalls++
	r.lastID = id
	r.lastReq = req
	if r.failIfInvoked != nil {
		r.failIfInvoked.Fatal("ReconnectSession must not be called; validation should reject the request first")
	}
	if r.reconnectErr != nil {
		return nil, r.reconnectErr
	}
	if r.reconnectResp != nil {
		return r.reconnectResp, nil
	}
	return &afclient.ReconnectSessionResponse{}, nil
}

func TestAgentReconnectHelp(t *testing.T) {
	t.Parallel()

	mock := afclient.NewMockClient()
	ds := func() afclient.DataSource { return mock }
	cmd, buf := newTestAgentCmd(ds, []string{"reconnect", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"<session-id>", "--cursor", "--last-event-id", "--json"} {
		if !strings.Contains(out, want) {
			t.Errorf("reconnect --help missing %q; got:\n%s", want, out)
		}
	}
}

func TestAgentParentHelpListsReconnect(t *testing.T) {
	t.Parallel()

	mock := afclient.NewMockClient()
	ds := func() afclient.DataSource { return mock }
	cmd, buf := newTestAgentCmd(ds, []string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "reconnect") {
		t.Errorf("agent --help missing reconnect subcommand; got:\n%s", buf.String())
	}
}

func TestAgentReconnectArgValidation(t *testing.T) {
	t.Parallel()

	mock := afclient.NewMockClient()
	ds := func() afclient.DataSource { return mock }

	tests := []struct {
		name string
		args []string
	}{
		{name: "zero_args", args: []string{"reconnect"}},
		{name: "two_args", args: []string{"reconnect", "a", "b"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd, _ := newTestAgentCmd(ds, tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected arg validation error, got nil")
			}
			if !strings.Contains(err.Error(), "accepts 1 arg") {
				t.Errorf("expected cobra ExactArgs(1) error; got: %v", err)
			}
		})
	}
}

func TestAgentReconnectWhitespaceSessionIDSkipsRPC(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{failIfInvoked: t}
	cmd, _ := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "   "})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty session id, got nil")
	}
	if !strings.Contains(err.Error(), "session id must not be empty") {
		t.Errorf("expected session id validation error; got: %v", err)
	}
	if ds.reconnectCalls != 0 {
		t.Errorf("ReconnectSession call count = %d, want 0", ds.reconnectCalls)
	}
}

func TestAgentReconnectMockHumanMode(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{
		reconnectResp: &afclient.ReconnectSessionResponse{
			Reconnected:   true,
			SessionID:     "SUP-674",
			SessionStatus: afclient.SessionStatus("running"),
			MissedEvents:  0,
		},
	}
	cmd, buf := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "SUP-674"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	const want = "reconnected to SUP-674 (status: running, missed: 0 events)\n"
	out := buf.String()
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if ds.lastID != "SUP-674" {
		t.Errorf("session id = %q, want %q", ds.lastID, "SUP-674")
	}
}

func TestAgentReconnectMockJSONMode(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{
		reconnectResp: &afclient.ReconnectSessionResponse{
			Reconnected:   true,
			SessionID:     "SUP-674",
			SessionStatus: afclient.SessionStatus("running"),
			MissedEvents:  0,
		},
	}
	cmd, buf := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "--json", "SUP-674"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	var resp agentReconnectJSON
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if !resp.Reconnected || resp.SessionID != "SUP-674" || resp.SessionStatus != afclient.SessionStatus("running") || resp.MissedEvents != 0 {
		t.Errorf("unexpected JSON payload: %+v", resp)
	}
	if !strings.Contains(out, "\n  \"sessionId\"") {
		t.Errorf("expected indented JSON output; got:\n%s", out)
	}
}

func TestAgentReconnectPassesResumeHints(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{
		reconnectResp: &afclient.ReconnectSessionResponse{
			Reconnected:   true,
			SessionID:     "SUP-674",
			SessionStatus: afclient.StatusWorking,
			MissedEvents:  2,
		},
	}
	cmd, _ := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "--cursor", "2026-04-15T10:00:00Z", "--last-event-id", "evt_abc123", "SUP-674"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if ds.lastReq.Cursor == nil || *ds.lastReq.Cursor != "2026-04-15T10:00:00Z" {
		t.Fatalf("cursor = %#v, want set", ds.lastReq.Cursor)
	}
	if ds.lastReq.LastEventID == nil || *ds.lastReq.LastEventID != "evt_abc123" {
		t.Fatalf("lastEventID = %#v, want set", ds.lastReq.LastEventID)
	}
}

func TestAgentReconnectNotFoundPropagation(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{reconnectErr: afclient.ErrNotFound}
	cmd, _ := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "SUP-674"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, afclient.ErrNotFound) {
		t.Errorf("errors.Is(err, afclient.ErrNotFound) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "reconnect SUP-674") {
		t.Errorf("expected reconnect prefix; got: %v", err)
	}
}

func TestAgentReconnectDeclined(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{
		reconnectResp: &afclient.ReconnectSessionResponse{
			Reconnected:   false,
			SessionID:     "SUP-674",
			SessionStatus: afclient.StatusStopped,
			MissedEvents:  3,
		},
	}
	cmd, _ := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "SUP-674"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected declined reconnect error, got nil")
	}
	if !strings.Contains(err.Error(), "reconnect declined for SUP-674") {
		t.Errorf("expected declined reconnect error; got: %v", err)
	}
}

func TestAgentReconnectPluralization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		resp afclient.ReconnectSessionResponse
		want string
	}{
		{
			name: "zero",
			resp: afclient.ReconnectSessionResponse{Reconnected: true, SessionID: "SUP-674", SessionStatus: afclient.StatusWorking, MissedEvents: 0},
			want: "reconnected to SUP-674 (status: working, missed: 0 events)\n",
		},
		{
			name: "one",
			resp: afclient.ReconnectSessionResponse{Reconnected: true, SessionID: "SUP-674", SessionStatus: afclient.StatusWorking, MissedEvents: 1},
			want: "reconnected to SUP-674 (status: working, missed: 1 event)\n",
		},
		{
			name: "many",
			resp: afclient.ReconnectSessionResponse{Reconnected: true, SessionID: "SUP-674", SessionStatus: afclient.StatusWorking, MissedEvents: 7},
			want: "reconnected to SUP-674 (status: working, missed: 7 events)\n",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ds := &reconnectStubDataSource{reconnectResp: &tt.resp}
			cmd, buf := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "SUP-674"})
			err := cmd.Execute()
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			out := buf.String()
			if out != tt.want {
				t.Errorf("stdout = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestAgentReconnectServerErrorPropagation(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{reconnectErr: afclient.ErrServerError}
	cmd, _ := newTestAgentCmd(func() afclient.DataSource { return ds }, []string{"reconnect", "sess-1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected server error, got nil")
	}
	if !errors.Is(err, afclient.ErrServerError) {
		t.Errorf("expected errors.Is(err, afclient.ErrServerError); got: %v", err)
	}
	if !strings.Contains(err.Error(), "reconnect sess-1") {
		t.Errorf("expected reconnect prefix; got: %v", err)
	}
}
