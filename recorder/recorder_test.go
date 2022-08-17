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
	"time"

	"gopkg.in/dnaeon/go-vcr.v2/cassette"
	"gopkg.in/dnaeon/go-vcr.v2/recorder"
)

type testCase struct {
	method            string
	body              string
	wantBody          string
	wantStatus        int
	wantContentLength int
}

func (tc testCase) run(client *http.Client, ctx context.Context, url string) error {
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

// newTimestampId returns a new ID based on the current timestamp
func newTimestampId() string {
	return time.Now().Format(time.RFC3339Nano)
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

// newHttpClient creates a new test HTTP client
func newHttpClient(r *recorder.Recorder) *http.Client {
	client := &http.Client{
		Transport: r, // Our recorder's transport
	}

	return client
}

func TestRecordingMode(t *testing.T) {
	// Set things up
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
		},
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_record")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.NewAsMode(cassPath, recorder.ModeRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecording {
		t.Fatalf("recorder should be in ModeRecording, got %q", rec.Mode())
	}

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
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
	client = newHttpClient(rec)

	if rec.Mode() != recorder.ModeReplayingOrRecording {
		t.Fatalf("recorder should be in ModeReplayingOrRecording, got %q", rec.Mode())
	}

	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplayingModeFailsWithEmptyCassette(t *testing.T) {
	cassPath, err := newCassettePath("replaying_mode_fails_with_empty_cassette")
	if err != nil {
		t.Fatal(err)
	}

	_, err = recorder.NewAsMode(cassPath, recorder.ModeReplaying, nil)
	if err != cassette.ErrCassetteNotFound {
		t.Fatalf("expected cassette.ErrCassetteNotFound, got %v", err)
	}
}

func TestWithContextTimeout(t *testing.T) {
	cassPath, err := newCassettePath("record_playback_timeout")
	if err != nil {
		t.Fatal(err)
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	rec, err := recorder.NewAsMode(cassPath, recorder.ModeReplayingOrRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
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
	defer rec.Stop()
	client = newHttpClient(rec)

	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		cancelFn()
		err = test.run(client, ctx, serverUrl)
		if err == nil {
			t.Fatalf("Expected cancellation error, got %v", err)
		}
	}
}

func TestModePlaybackMissingEpisodes(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("record_playback_missing_episodes")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.NewAsMode(cassPath, recorder.ModeReplayingOrRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeReplayingOrRecording {
		t.Fatalf("recorder should be in ModeReplayingOrRecording, got %q", rec.Mode())
	}

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run again with new HTTP interactions
	server.Close()
	rec.Stop()

	rec, err = recorder.NewAsMode(cassPath, recorder.ModeReplaying, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()

	if rec.Mode() != recorder.ModeReplaying {
		t.Fatalf("recorder should be in ModeReplaying, got %q", rec.Mode())
	}

	newTests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
		},
		{
			method:            http.MethodPost,
			body:              "bar",
			wantBody:          "POST go-vcr\nbar",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	client = newHttpClient(rec)
	for _, test := range newTests {
		err := test.run(client, ctx, serverUrl)
		urlErr, ok := err.(*url.Error)
		if !ok {
			t.Fatalf("Expected err but was %T %s", err, err)
		}
		if urlErr.Err != cassette.ErrInteractionNotFound {
			t.Fatalf("Expected cassette.ErrInteractionNotFound but was %T %s", err, err)
		}
	}
}

func TestModeDisabled(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("recorder_disabled_mode")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := recorder.NewAsMode(cassPath, recorder.ModeDisabled, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()
	defer server.Close()

	if m := rec.Mode(); m != recorder.ModeDisabled {
		t.Fatalf("Expected recorder in ModeDisabled, got %q", m)
	}

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
	for _, test := range tests {
		if err := test.run(client, ctx, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Expect the file to not exist if record is disabled
	if _, err := cassette.Load(cassPath); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPassthroughMode(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodGet,
			wantBody:          "GET go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 11,
		},
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
		},
		{
			method:            http.MethodPost,
			body:              "passthrough request",
			wantBody:          "POST go-vcr\npassthrough request",
			wantStatus:        http.StatusOK,
			wantContentLength: 31,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_passthrough_mode")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.NewAsMode(cassPath, recorder.ModeRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecording {
		t.Fatalf("recorder should be in ModeRecording, got %q", rec.Mode())
	}

	// Add a passthrough configuration which does not record any requests with
	// a specific body.
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
	client := newHttpClient(rec)
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
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_filter")
	if err != nil {
		t.Fatal(err)
	}

	// Create recorder
	rec, err := recorder.NewAsMode(cassPath, recorder.ModeRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecording {
		t.Fatalf("recorder should be in ModeRecording, got %q", rec.Mode())
	}

	// Add a filter which replaces each request body in the stored cassette:
	dummyBody := "[REDACTED]"
	rec.AddFilter(func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	})

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
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

func TestSaveFilter(t *testing.T) {
	tests := []testCase{
		{
			method:            http.MethodHead,
			wantStatus:        http.StatusOK,
			wantContentLength: 12,
		},
		{
			method:            http.MethodPost,
			body:              "foo",
			wantBody:          "POST go-vcr\nfoo",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath, err := newCassettePath("test_save_filter")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := recorder.NewAsMode(cassPath, recorder.ModeRecording, nil)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecording {
		t.Fatalf("recorder should be in ModeRecording, got %q", rec.Mode())
	}

	dummyBody := "[REDACTED]"

	// Add a save filter which replaces each request body in the stored cassette
	rec.AddSaveFilter(func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	})

	// Run tests
	ctx := context.Background()
	client := newHttpClient(rec)
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
