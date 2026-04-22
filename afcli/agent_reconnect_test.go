package afcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/RenseiAI/agentfactory-tui/afclient"
)

type reconnectStubDataSource struct {
	stubDataSource
	reconnectResp  *afclient.ReconnectSessionResponse
	reconnectErr   error
	reconnectCalls int
	reconnectID    string
	reconnectReq   afclient.ReconnectSessionRequest
	failIfInvoked  *testing.T
}

func (r *reconnectStubDataSource) ReconnectSession(id string, req afclient.ReconnectSessionRequest) (*afclient.ReconnectSessionResponse, error) {
	r.reconnectCalls++
	r.reconnectID = id
	r.reconnectReq = req
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

func runReconnectWithStub(t *testing.T, ds afclient.DataSource, args []string) (string, error) {
	t.Helper()

	cmd := newAgentReconnectCmd(func() afclient.DataSource { return ds })
	cmd.SilenceErrors = true

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
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
				t.Fatal("expected error for wrong arg count, got nil")
			}
			if !strings.Contains(err.Error(), "accepts 1 arg") {
				t.Errorf("expected cobra ExactArgs(1) error; got: %v", err)
			}
		})
	}
}

func TestAgentReconnectWhitespaceIDSkipsRPC(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{failIfInvoked: t}
	_, err := runReconnectWithStub(t, ds, []string{"   "})
	if err == nil {
		t.Fatal("expected error for empty session id, got nil")
	}
	if !strings.Contains(err.Error(), "session id must not be empty") {
		t.Errorf("expected empty session id error; got: %v", err)
	}
	if ds.reconnectCalls != 0 {
		t.Errorf("ReconnectSession call count = %d, want 0", ds.reconnectCalls)
	}
}

func TestAgentReconnectMockHumanMode(t *testing.T) {
	t.Parallel()

	mock := afclient.NewMockClient()
	ds := func() afclient.DataSource { return mock }
	cmd, buf := newTestAgentCmd(ds, []string{"reconnect", "mock-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	const want = "reconnected to mock-001 (status: working, missed: 0 events)\n"
	if got := buf.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestAgentReconnectMockJSONMode(t *testing.T) {
	t.Parallel()

	mock := afclient.NewMockClient()
	ds := func() afclient.DataSource { return mock }
	cmd, buf := newTestAgentCmd(ds, []string{"reconnect", "--json", "mock-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var resp afclient.ReconnectSessionResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if !resp.Reconnected || resp.SessionID != "mock-001" || resp.SessionStatus != afclient.StatusWorking || resp.MissedEvents != 0 {
		t.Errorf("unexpected reconnect response: %+v", resp)
	}
	if !strings.Contains(buf.String(), "\n  \"reconnected\"") {
		t.Errorf("expected indented JSON output; got:\n%s", buf.String())
	}
}

func TestAgentReconnectForwardsOptionalFlags(t *testing.T) {
	t.Parallel()

	cursor := "2026-04-15T10:00:00Z"
	lastEventID := "evt_abc123"

	tests := []struct {
		name       string
		args       []string
		wantCursor *string
		wantLastID *string
	}{
		{name: "none", args: []string{"mock-001"}},
		{name: "cursor_only", args: []string{"--cursor", cursor, "mock-001"}, wantCursor: &cursor},
		{name: "last_event_only", args: []string{"--last-event-id", lastEventID, "mock-001"}, wantLastID: &lastEventID},
		{name: "both", args: []string{"--cursor", cursor, "--last-event-id", lastEventID, "mock-001"}, wantCursor: &cursor, wantLastID: &lastEventID},
		{name: "explicit_empty_cursor", args: []string{"--cursor", "", "mock-001"}, wantCursor: strPtr("")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ds := &reconnectStubDataSource{
				reconnectResp: &afclient.ReconnectSessionResponse{
					Reconnected:   true,
					SessionID:     "mock-001",
					SessionStatus: afclient.StatusWorking,
					MissedEvents:  0,
				},
			}
			if _, err := runReconnectWithStub(t, ds, tt.args); err != nil {
				t.Fatalf("execute: %v", err)
			}

			if ds.reconnectID != "mock-001" {
				t.Errorf("ReconnectSession id = %q, want %q", ds.reconnectID, "mock-001")
			}
			assertStringPtrEqual(t, "cursor", ds.reconnectReq.Cursor, tt.wantCursor)
			assertStringPtrEqual(t, "lastEventId", ds.reconnectReq.LastEventID, tt.wantLastID)
		})
	}
}

func TestAgentReconnectPluralization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		missedEvents int
		want         string
	}{
		{name: "zero", missedEvents: 0, want: "missed: 0 events"},
		{name: "one", missedEvents: 1, want: "missed: 1 event"},
		{name: "many", missedEvents: 7, want: "missed: 7 events"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ds := &reconnectStubDataSource{
				reconnectResp: &afclient.ReconnectSessionResponse{
					Reconnected:   true,
					SessionID:     "mock-001",
					SessionStatus: afclient.StatusWorking,
					MissedEvents:  tt.missedEvents,
				},
			}
			out, err := runReconnectWithStub(t, ds, []string{"mock-001"})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output %q missing %q", out, tt.want)
			}
		})
	}
}

func TestPluralizeEvents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		n    int
		want string
	}{
		{n: 0, want: "events"},
		{n: 1, want: "event"},
		{n: 2, want: "events"},
	}

	for _, tc := range cases {
		if got := pluralizeEvents(tc.n); got != tc.want {
			t.Errorf("pluralizeEvents(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestAgentReconnectDeclined(t *testing.T) {
	t.Parallel()

	ds := &reconnectStubDataSource{
		reconnectResp: &afclient.ReconnectSessionResponse{
			Reconnected:   false,
			SessionID:     "mock-001",
			SessionStatus: afclient.StatusStopped,
			MissedEvents:  2,
		},
	}
	out, err := runReconnectWithStub(t, ds, []string{"mock-001"})
	if err == nil {
		t.Fatal("expected error for declined reconnect, got nil")
	}
	if !strings.Contains(err.Error(), "reconnect declined for mock-001 (status: stopped, missed: 2 events)") {
		t.Errorf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty output", out)
	}
}

func TestAgentReconnectSentinelPropagation(t *testing.T) {
	t.Parallel()

	for _, wantErr := range []error{
		afclient.ErrNotFound,
		afclient.ErrNotAuthenticated,
		afclient.ErrUnauthorized,
		afclient.ErrServerError,
	} {
		wantErr := wantErr
		t.Run(wantErr.Error(), func(t *testing.T) {
			t.Parallel()

			ds := &reconnectStubDataSource{reconnectErr: wantErr}
			_, err := runReconnectWithStub(t, ds, []string{"mock-001"})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, wantErr) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", wantErr, err)
			}
			if !strings.Contains(err.Error(), "reconnect session") {
				t.Errorf("expected wrapped reconnect session context; got: %v", err)
			}
		})
	}
}

func TestAgentReconnectHTTPNotFound(t *testing.T) {
	t.Parallel()

	client := afclient.NewClient("http://coordinator.test")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("missing")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	ds := func() afclient.DataSource { return client }
	cmd, _ := newTestAgentCmd(ds, []string{"reconnect", "sess-unknown"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from 404, got nil")
	}
	if !errors.Is(err, afclient.ErrNotFound) {
		t.Errorf("expected errors.Is(err, afclient.ErrNotFound); got: %v", err)
	}
	if !strings.Contains(err.Error(), "reconnect session") {
		t.Errorf("expected wrapped reconnect session error; got: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func strPtr(s string) *string {
	return &s
}

func assertStringPtrEqual(t *testing.T, name string, got, want *string) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil && want != nil:
		t.Errorf("%s = nil, want %q", name, *want)
	case got != nil && want == nil:
		t.Errorf("%s = %q, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s = %q, want %q", name, *got, *want)
	}
}
