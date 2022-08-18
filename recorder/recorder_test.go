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
	defer server.Close()

	if m := rec.Mode(); m != recorder.ModePassthrough {
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

func TestFilter(t *testing.T) {
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

	cassPath, err := newCassettePath("test_filter")
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

	// Add a filter which replaces each request body in the stored
	// cassette:
	dummyBody := "[REDACTED]"
	rec.AddFilter(func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	})

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the filter has been applied
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

func TestPreSaveFilter(t *testing.T) {
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

	cassPath, err := newCassettePath("test_pre_save_filter")
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

	// Add a save filter which replaces each request body in the stored cassette
	dummyBody := "[REDACTED]"
	rec.AddPreSaveFilter(func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	})

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the filter has been applied
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
