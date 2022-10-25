// Copyright (c) 2015-2016 Marin Atanasov Nikolov <dnaeon@gmail.com>
// Copyright (c) 2016 David Jack <davars@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer
//    in this position and unchanged.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE AUTHOR(S) ``AS IS'' AND ANY EXPRESS OR
// IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
// IN NO EVENT SHALL THE AUTHOR(S) BE LIABLE FOR ANY DIRECT, INDIRECT,
// INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
// NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
// THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package recorder_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v3/cassette"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

type testCase struct {
	method            string
	body              string
	wantBody          string
	wantStatus        int
	wantContentLength int
	path              string
}

func (tc testCase) run(client *http.Client, ctx context.Context, serverUrl string) error {
	url := fmt.Sprintf("%s%s", serverUrl, tc.path)
	req, err := http.NewRequest(tc.method, url, strings.NewReader(tc.body))
	if err != nil {
		return err
	}

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if string(respBody) != tc.wantBody {
		return fmt.Errorf("got body: %q, want body: %q", string(respBody), tc.wantBody)
	}

	if resp.StatusCode != tc.wantStatus {
		return fmt.Errorf("want status: %q, got status: %q", resp.StatusCode, tc.wantStatus)
	}

	if resp.ContentLength != int64(tc.wantContentLength) {
		return fmt.Errorf("want ContentLength %d, got %d", tc.wantContentLength, resp.ContentLength)
	}

	return nil
}

// newEchoHttpServer creates a new HTTP server for testing purposes
func newEchoHttpServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s go-vcr", r.Method)
		if r.Body != nil {
			defer r.Body.Close()
			fmt.Fprintln(w)
			io.Copy(w, r.Body)
		}
	})
	server := httptest.NewServer(handler)

	return server
}

// newCassettePath creates a new path to be used for test cassettes,
// which reside in a temporary location.
func newCassettePath(name string) (string, error) {
	dir, err := ioutil.TempDir(os.TempDir(), "go-vcr-")
	if err != nil {
		return "", err
	}
	cassPath := path.Join(dir, name)

	return cassPath, nil
}

func TestRecordOnceMode(t *testing.T) {
	// Set things up
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/bar",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/baz",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/qux",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_record")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	if !rec.IsNewCassette() {
		t.Fatal("recorder is not using a new cassette")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Stop server and recorder, then re-run the tests without server
	server.Close()
	rec.Stop()

	// Verify we've got correct interactions recorded in the cassette
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify cassette contents
	if len(tests) != len(c.Interactions) {
		t.Fatalf("expected %d recorded interactions, got %d", len(tests), len(c.Interactions))
	}

	for i, test := range tests {
		recordedBody := c.Interactions[i].Request.Body
		if test.body != recordedBody {
			t.Fatalf("got recorded body: %q, want recorded body: %q", test.body, recordedBody)
		}

		recordedMethod := c.Interactions[i].Request.Method
		if test.method != recordedMethod {
			t.Fatalf("got recorded method: %q, want recorded method: %q", test.method, recordedMethod)
		}

		recordedStatus := c.Interactions[i].Response.Code
		if test.wantStatus != recordedStatus {
			t.Fatalf("got recorded status: %q, want recorded status: %q", test.wantStatus, recordedStatus)

		}
	}

	// Re-run without the actual server
	rec, err = recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()
	client = rec.GetDefaultClient()

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplayOnlyModeFailsWithMissingCassette(t *testing.T) {
	opts := &recorder.Options{
		CassetteName: "missing_cassette_file",
		Mode:         recorder.ModeReplayOnly,
	}
	_, err := recorder.NewWithOptions(opts)
	if err != cassette.ErrCassetteNotFound {
		t.Fatalf("expected cassette.ErrCassetteNotFound, got %v", err)
	}
}

func TestReplayWithContextTimeout(t *testing.T) {
	cassPath, err := newCassettePath("test_record_playback_timeout")
	if err != nil {
		t.Fatal(err)
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/path",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run without the actual server
	server.Close()
	rec.Stop()

	rec, err = recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	// This time the recording should only be replaying
	if rec.IsRecording() != false {
		t.Fatal("recorder should not be recording")
	}

	defer rec.Stop()
	client = rec.GetDefaultClient()

	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		cancelFn()
		err = test.run(client, ctx, serverUrl)
		if err == nil {
			t.Fatalf("expected cancellation error, got %v", err)
		}
	}
}

func TestRecordOnceWithMissingEpisodes(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_record_playback_missing_episodes")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run again with new HTTP interactions
	server.Close()
	rec.Stop()

	rec, err = recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != false {
		t.Fatal("recorder should not be recording")
	}

	newTests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/new-path-here",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/and-another-one-goes-here",
		},
	}

	// New episodes should return errors
	client = rec.GetDefaultClient()
	for _, test := range newTests {
		err := test.run(client, ctx, serverUrl)
		urlErr, ok := err.(*url.Error)
		if !ok {
			t.Fatalf("expected err but was %T %s", err, err)
		}
		if urlErr.Err != cassette.ErrInteractionNotFound {
			t.Fatalf("expected cassette.ErrInteractionNotFound but was %T %s", err, err)
		}
	}
}

func TestReplayWithNewEpisodes(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL
	defer server.Close()

	cassPath, err := newCassettePath("test_replay_with_missing_episodes")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	opts := &recorder.Options{
		CassetteName: cassPath,
		Mode:         recorder.ModeReplayWithNewEpisodes,
	}
	rec, err := recorder.NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	if rec.Mode() != recorder.ModeReplayWithNewEpisodes {
		t.Fatal("recorder is not in the correct mode")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run again with new HTTP interactions
	rec.Stop()

	rec, err = recorder.NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeReplayWithNewEpisodes {
		t.Fatal("recorder is not in the correct mode")
	}

	newTests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/new-path-here",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/and-another-one-goes-here",
		},
	}

	// New episodes should be added to the cassette
	client = rec.GetDefaultClient()
	for _, test := range newTests {
		err := test.run(client, ctx, serverUrl)
		if err != nil {
			t.Fatalf("expected to add new episode, got error: %s", err)
		}
	}

	// Verify cassette contents
	rec.Stop()

	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	total := len(tests) + len(newTests)
	if total != len(c.Interactions) {
		t.Fatalf("expected %d recorded interactions, got %d", total, len(c.Interactions))
	}
}

func TestPassthroughMode(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL
	defer server.Close()

	cassPath, err := newCassettePath("test_passthrough_mode")
	if err != nil {
		t.Fatal(err)
	}

	opts := &recorder.Options{
		CassetteName: cassPath,
		Mode:         recorder.ModePassthrough,
	}
	rec, err := recorder.NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	if m := rec.Mode(); m != recorder.ModePassthrough {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != false {
		t.Fatal("recorder should not be recording")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Expect the file to not exist if record is disabled
	rec.Stop()

	if _, err := cassette.Load(cassPath); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPassthroughHandler(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/bar",
		},
		{
			method:            http.MethodPost,
			body:              "passthrough request",
			wantBody:          "POST go-vcr\npassthrough request",
			wantStatus:        http.StatusOK,
			wantContentLength: 31,
			path:              "/api/v1/baz",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_passthrough_handler")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Add a passthrough handler which does not record any
	// requests with a specific body.
	rec.AddPassthrough(func(r *http.Request) bool {
		if r.Body == nil {
			return false
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(r.Body); err != nil {
			return false
		}
		r.Body = ioutil.NopCloser(&b)

		return r.Method == http.MethodPost && b.String() == "passthrough request"
	})

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the passthrough interaction is not recorded
	server.Close()
	rec.Stop()

	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// One interaction less should be recorded
	numRecorded := len(c.Interactions)
	numTests := len(tests)
	if numTests-1 != numRecorded {
		t.Fatalf("expected %d recorded interactions, got %d", numTests-1, numRecorded)
	}

	// Assert that no body exists matching our passthrough test
	for _, i := range c.Interactions {
		body := i.Request.Body
		if i.Request.Method == http.MethodPost && body == "passthrough request" {
			t.Fatalf("passthrough request should not be recorded: %q", body)
		}
	}
}

func TestAfterCaptureHook(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_after_capture_hook")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Add a hook which replaces each request body in the stored
	// cassette
	dummyBody := "[REDACTED]"
	redactHook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	}
	rec.AddHook(redactHook, recorder.AfterCaptureHook)

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the hooks has been applied
	server.Close()
	rec.Stop()

	// Load the cassette we just stored
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	for i := range tests {
		body := c.Interactions[i].Request.Body
		if c.Interactions[i].Request.Method == http.MethodPost && body != dummyBody {
			t.Fatalf("want body: %q, got body: %q", dummyBody, body)
		}
	}
}

func TestBeforeSaveHook(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_before_save_hook")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Add a hook which replaces each request body in the stored cassette
	dummyBody := "[REDACTED]"
	redactHook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	}
	rec.AddHook(redactHook, recorder.BeforeSaveHook)

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the hook has been applied
	server.Close()
	rec.Stop()

	// Load the cassette we just stored
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that each body has been set to our dummy value
	for i := range tests {
		body := c.Interactions[i].Request.Body
		if c.Interactions[i].Request.Method == http.MethodPost && body != dummyBody {
			t.Fatalf("want body: %q, got body: %q", dummyBody, body)
		}
	}
}

func TestBeforeResponseReplayHook(t *testing.T) {
	// Do initial recording of the interactions, then use a
	// BeforeResponseReplayHook to modify the body returned to the
	// client.
	tests := []testCase{
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_before_response_replay_hook")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Stop recorder and server. Then re-run the tests with a
	// BeforeResponseReplay hook installed, which will modify the
	// body of each response before returning it to the client.
	server.Close()
	rec.Stop()

	// Re-run the tests with the hook installed.
	rec, err = recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	// Recorder should not be recording this time
	if rec.IsRecording() != false {
		t.Fatal("recorder should not be recording")
	}

	// Add a hook which replaces each request body of a previously
	// recorded interaction.
	dummyBody := "MODIFIED BODY"
	hook := func(i *cassette.Interaction) error {
		i.Response.Body = dummyBody

		return nil
	}
	rec.AddHook(hook, recorder.BeforeResponseReplayHook)

	newTests := []testCase{
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          dummyBody,
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          dummyBody,
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	client = rec.GetDefaultClient()
	for _, test := range newTests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplayableInteractions(t *testing.T) {
	tc := testCase{
		method:            http.MethodGet,
		wantBody:          "GET go-vcr\n",
		wantStatus:        http.StatusOK,
		wantContentLength: 11,
		path:              "/api/v1/foo",
	}

	server := newEchoHttpServer()
	serverUrl := server.URL
	defer server.Close()

	cassPath, err := newCassettePath("test_replayable_interactions")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Configure replayable interactions
	rec.SetReplayableInteractions(true)

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for i := 0; i < 10; i++ {
		if err := tc.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// We should have only 1 interaction recorded
	rec.Stop()

	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	total := len(c.Interactions)
	if total != 1 {
		t.Fatalf("expected 1 recorded interaction, got %d", total)
	}
}

func TestWithCustomMatcher(t *testing.T) {
	// Setup test cases with same method and API path, but with
	// different bodies.
	tests := []testCase{
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo", // Same endpoint
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo", // Same endpoint
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_with_custom_matcher")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// Run tests first in RecordOnce mode, so we capture the
	// interactions
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Stop recorder and server, then re-run tests in ReplayOnly
	// mode. During this test we expect that the default matcher
	// will fail for the requests with same method and API path,
	// since it does not match on the request body. It will always
	// return the body of the first recorded interaction for the
	// same endpoint.
	server.Close()
	rec.Stop()

	opts := &recorder.Options{
		CassetteName: cassPath,
		Mode:         recorder.ModeReplayOnly,
	}
	rec, err = recorder.NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()

	if rec.Mode() != recorder.ModeReplayOnly {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != false {
		t.Fatal("recorder should not be recording")
	}

	// Set replayable interactions to true, so that we can match
	// against the already recorded interactions.
	rec.SetReplayableInteractions(true)

	// All requests which hit the same URL and use the same method
	// will match against the first recorded interaction.
	client = rec.GetDefaultClient()
	url := fmt.Sprintf("%s%s", serverUrl, "/api/v1/foo") // Same URL as the test cases
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("any body will match"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Body should be the same as the first recorded interaction
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	wantBody := tests[0].wantBody
	if string(respBody) != wantBody {
		t.Fatalf("got body: %q, want body: %q", string(respBody), wantBody)
	}

	// Now configure our custom matcher, which should match
	// against the body as well
	customMatcher := func(r *http.Request, i cassette.Request) bool {
		if r.Body == nil || r.Body == http.NoBody {
			return cassette.DefaultMatcher(r, i)
		}

		var reqBody []byte
		var err error
		reqBody, err = ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal("failed to read request body")
		}
		r.Body.Close()
		r.Body = ioutil.NopCloser(bytes.NewBuffer(reqBody))

		return r.Method == i.Method && r.URL.String() == i.URL && string(reqBody) == i.Body
	}

	// Re-running same tests should complete fine this time when
	// using the custom matcher.
	rec.SetMatcher(customMatcher)

	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecordOnlyMode(t *testing.T) {
	// Set things up
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/baz",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL
	defer server.Close()

	cassPath, err := newCassettePath("test_record_only")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	opts := &recorder.Options{
		CassetteName: cassPath,
		Mode:         recorder.ModeRecordOnly,
	}
	rec, err := recorder.NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()

	if rec.Mode() != recorder.ModeRecordOnly {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	if !rec.IsNewCassette() {
		t.Fatal("recorder is not using a new cassette")
	}

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInvalidRecorderMode(t *testing.T) {
	// Create recorder
	opts := &recorder.Options{
		CassetteName: "invalid_recorder_mode",
		Mode:         recorder.Mode(-42),
	}
	_, err := recorder.NewWithOptions(opts)
	if err != recorder.ErrInvalidMode {
		t.Fatal("expected recorder to fail with invalid mode")
	}
}

func TestDiscardInteractionsOnSave(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/bar",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_discard_interactions_on_save")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	// The following hook function will be used to determine
	// whether an interaction is to be discarded when saving the
	// cassette on disk.
	hook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.DiscardOnSave = true
		}

		return nil
	}
	rec.AddHook(hook, recorder.AfterCaptureHook)

	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Stop recorder and verify cassette
	rec.Stop()

	cass, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// We should have one interaction less than our test cases
	// when reading the cassette from disk.
	wantInteractions := len(tests) - 1
	gotInteractions := len(cass.Interactions)
	if wantInteractions != gotInteractions {
		t.Fatalf("expected %d interactions, got %d", wantInteractions, gotInteractions)
	}
}
