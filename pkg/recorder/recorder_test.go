// Copyright (c) 2015-2024 Marin Atanasov Nikolov <dnaeon@gmail.com>
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

type testCase struct {
	method            string
	body              string
	wantBody          string
	wantStatus        int
	wantContentLength int
	wantError         error
	path              string
}

func (tc testCase) run(ctx context.Context, client *http.Client, serverUrl string) error {
	url := fmt.Sprintf("%s%s", serverUrl, tc.path)
	req, err := http.NewRequest(tc.method, url, strings.NewReader(tc.body))
	if err != nil {
		return err
	}

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		if tc.wantError != nil && errors.Is(err, tc.wantError) {
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
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

func TestRecordOnceMode(t *testing.T) {
	t.Parallel()
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
		{
			method:            http.MethodPut,
			body:              "foobar",
			wantBody:          "PUT go-vcr\nfoobar",
			wantStatus:        http.StatusOK,
			wantContentLength: 17,
			path:              "/api/v1/foobar?foo=bar&baz=qux",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL

	cassPath := t.TempDir()

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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplayOnlyModeFailsWithMissingCassette(t *testing.T) {
	t.Parallel()
	opts := []recorder.Option{
		recorder.WithMode(recorder.ModeReplayOnly),
	}
	_, err := recorder.New("missing_cassette_file", opts...)
	if !errors.Is(err, cassette.ErrCassetteNotFound) {
		t.Fatalf("expected cassette.ErrCassetteNotFound, got %v", err)
	}
}

func TestReplayWithContextTimeout(t *testing.T) {
	t.Parallel()
	cassPath := t.TempDir()
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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
		err = test.run(ctx, client, serverUrl)
		if err == nil {
			t.Fatalf("expected cancellation error, got %v", err)
		}
	}
}

func TestRecordOnceWithMissingEpisodes(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
		err := test.run(ctx, client, serverUrl)
		urlErr, ok := err.(*url.Error)
		if !ok {
			t.Fatalf("expected err but was %T %s", err, err)
		}
		if !errors.Is(urlErr.Err, cassette.ErrInteractionNotFound) {
			t.Fatalf("expected cassette.ErrInteractionNotFound but was %T %s", err, err)
		}
	}
}

func TestReplayWithNewEpisodes(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder
	opts := []recorder.Option{
		recorder.WithMode(recorder.ModeReplayWithNewEpisodes),
	}
	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run again with new HTTP interactions
	rec.Stop()
	rec, err = recorder.New(cassPath, opts...)
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
		err := test.run(ctx, client, serverUrl)
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
	t.Parallel()
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
	cassPath := t.TempDir()

	opts := []recorder.Option{
		recorder.WithMode(recorder.ModePassthrough),
	}
	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// The file should not exists, since we haven't been recording
	rec.Stop()

	if _, err := cassette.Load(cassPath); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPassthroughHandler(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder
	opts := []recorder.Option{
		recorder.WithPassthrough(func(r *http.Request) bool {
			// Passthrough requests with POST method and "passthrough request" body
			if r.Body == nil {
				return false
			}
			var b bytes.Buffer
			if _, err := b.ReadFrom(r.Body); err != nil {
				return false
			}
			r.Body = io.NopCloser(&b)

			return r.Method == http.MethodPost && b.String() == "passthrough request"
		}),
	}
	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder and add a hook which replaces each request body in
	// the stored cassette
	dummyBody := "[REDACTED]"
	redactHook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	}

	opts := []recorder.Option{
		recorder.WithHook(redactHook, recorder.AfterCaptureHook),
	}

	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
	t.Parallel()
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
	cassPath := t.TempDir()

	// Add a hook which replaces each request body in the stored cassette
	dummyBody := "[REDACTED]"
	redactHook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.Request.Body = dummyBody
		}
		return nil
	}
	opts := []recorder.Option{
		recorder.WithHook(redactHook, recorder.BeforeSaveHook),
	}
	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
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
	t.Parallel()
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
	cassPath := t.TempDir()

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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// Stop recorder and server. Then re-run the tests with a
	// BeforeResponseReplay hook installed, which will modify the
	// body of each response before returning it to the client.
	server.Close()
	rec.Stop()

	// Re-run the tests with the hook installed.  Add a hook which replaces
	// each request body of a previously recorded interaction.
	dummyBody := "MODIFIED BODY"
	hook := func(i *cassette.Interaction) error {
		i.Response.Body = dummyBody

		return nil
	}

	opts := []recorder.Option{
		recorder.WithHook(hook, recorder.BeforeResponseReplayHook),
	}
	rec, err = recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReplayableInteractions(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder and use an on-recorder-stop hook to verify that
	// interactions were replayed
	errNotReplayed := errors.New("interaction was not replayed")
	hook := func(i *cassette.Interaction) error {
		if !i.WasReplayed() {
			return errNotReplayed
		}
		return nil
	}

	opts := []recorder.Option{
		recorder.WithReplayableInteractions(true),
		recorder.WithHook(hook, recorder.OnRecorderStopHook),
	}

	rec, err := recorder.New(cassPath, opts...)
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
	for i := 0; i < 10; i++ {
		if err := tc.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}

	// We should have only 1 interaction recorded after stopping the recorder
	if err := rec.Stop(); err != nil {
		t.Fatalf("recorder did not stop properly: %s", err)
	}

	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	total := len(c.Interactions)
	if total != 1 {
		t.Fatalf("expected 1 recorded interaction, got %d", total)
	}
}

func TestRecordOnlyMode(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder
	opts := []recorder.Option{
		recorder.WithMode(recorder.ModeRecordOnly),
	}
	rec, err := recorder.New(cassPath, opts...)
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
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBlockRealTransportUnsafeMethods(t *testing.T) {
	t.Parallel()
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
			method:            http.MethodOptions,
			wantBody:          "OPTIONS go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 15,
			path:              "/api/v1/foo",
		},
		{
			method:            http.MethodTrace,
			wantBody:          "TRACE go-vcr\n",
			wantStatus:        http.StatusOK,
			wantContentLength: 13,
			path:              "/api/v1/foo",
		},
		{
			method:    http.MethodPost,
			body:      "foo",
			wantError: recorder.ErrUnsafeRequestMethod,
			path:      "/api/v1/baz",
		},
		{
			method:    http.MethodPut,
			body:      "foo",
			wantError: recorder.ErrUnsafeRequestMethod,
			path:      "/api/v1/baz",
		},
		{
			method:    http.MethodDelete,
			wantError: recorder.ErrUnsafeRequestMethod,
			path:      "/api/v1/baz",
		},
		{
			method:    http.MethodConnect,
			wantError: recorder.ErrUnsafeRequestMethod,
			path:      "/api/v1/baz",
		},
		{
			method:    http.MethodPatch,
			body:      "foo",
			wantError: recorder.ErrUnsafeRequestMethod,
			path:      "/api/v1/baz",
		},
	}

	server := newEchoHttpServer()
	serverUrl := server.URL
	defer server.Close()
	cassPath := t.TempDir()

	// Create recorder
	opts := []recorder.Option{
		recorder.WithMode(recorder.ModeRecordOnly),
		recorder.WithBlockUnsafeMethods(true),
	}
	rec, err := recorder.New(cassPath, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Stop()

	// Run tests
	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(ctx, client, serverUrl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInvalidRecorderMode(t *testing.T) {
	t.Parallel()
	// Create recorder
	opts := []recorder.Option{
		recorder.WithMode(recorder.Mode(-42)),
	}
	_, err := recorder.New("invalid_recorder_mode", opts...)
	if err != recorder.ErrInvalidMode {
		t.Fatal("expected recorder to fail with invalid mode")
	}
}

func TestDiscardInteractionsOnSave(t *testing.T) {
	t.Parallel()
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
	cassPath := t.TempDir()

	// Create recorder and use a hook, which will be used to determine
	// whether an interaction is to be discarded when saving the cassette on
	// disk.
	hook := func(i *cassette.Interaction) error {
		if i.Request.Method == http.MethodPost && i.Request.Body == "foo" {
			i.DiscardOnSave = true
		}

		return nil
	}
	opts := []recorder.Option{
		recorder.WithHook(hook, recorder.AfterCaptureHook),
	}

	rec, err := recorder.New(cassPath, opts...)
	if err != nil {
		t.Fatal(err)
	}

	if rec.Mode() != recorder.ModeRecordOnce {
		t.Fatal("recorder is not in the correct mode")
	}

	if rec.IsRecording() != true {
		t.Fatal("recorder is not recording")
	}

	ctx := context.Background()
	client := rec.GetDefaultClient()
	for _, test := range tests {
		if err := test.run(ctx, client, serverUrl); err != nil {
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

func TestRecordAndPlaybackWithQueryParams(t *testing.T) {
	t.Parallel()
	testURL := ""
	cassPath := t.TempDir()

	// Make POST request with query parameters ?foo=bar&baz=true
	doRequest := func(c *http.Client) {
		req, err := http.NewRequest(http.MethodPost, testURL, strings.NewReader(`{"test-post-data":true}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if want := "Method: POST, foo: bar, baz: true"; string(respBody) != want {
			t.Fatalf("Recording phase: expected body %q, got %q", want, string(respBody))
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Recording phase: expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
	}

	// Phase 1: create the recording
	func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			queryParams := r.URL.Query()
			foo := queryParams.Get("foo")
			baz := queryParams.Get("baz")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Method: %s, foo: %s, baz: %s", r.Method, foo, baz)
			if r.Body == nil {
				t.Error("expected request body to be present")
				return
			}
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error("failed to read request body:", err)
				return
			}
			if string(body) != `{"test-post-data":true}` {
				t.Errorf("expected request body to be 'test-post-data', got '%s'", string(body))
				return
			}
		}))
		defer server.Close()

		// Phase 1: Record the interaction
		rec, err := recorder.New(cassPath, recorder.WithMode(recorder.ModeRecordOnce))
		if err != nil {
			t.Fatal(err)
		}
		if !rec.IsRecording() {
			t.Fatal("recorder should be in recording mode")
		}
		testURL = server.URL + "/test?foo=bar&baz=true"

		doRequest(rec.GetDefaultClient())

		// Stop the recorder to save the cassette
		err = rec.Stop()
		if err != nil {
			t.Fatal(err)
		}
		// Verify the cassette was created and contains the expected interaction
		c, err := cassette.Load(cassPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Interactions) != 1 {
			t.Fatalf("Expected 1 interaction, got %d", len(c.Interactions))
		}
		interaction := c.Interactions[0]
		if interaction.Request.Method != http.MethodPost {
			t.Fatalf("Expected POST method, got %s", interaction.Request.Method)
		}
		if interaction.Request.URL != testURL {
			t.Fatalf("Expected URL %s, got %s", testURL, interaction.Request.URL)
		}
		if want := `{"test-post-data":true}`; interaction.Request.Body != want {
			t.Fatalf("Expected request body %q, got %q", want, interaction.Request.Body)
		}
		if want := "Method: POST, foo: bar, baz: true"; interaction.Response.Body != want {
			t.Fatalf("Expected request body %q, got %q", want, interaction.Request.Body)
		}

		// Verify query parameters are recorded in the Form field
		form := interaction.Request.Form
		if form.Get("foo") != "bar" {
			t.Fatalf("Expected query param foo=bar, got %s", form.Get("foo"))
		}
		if form.Get("baz") != "true" {
			t.Fatalf("Expected query param baz=true, got %s", form.Get("baz"))
		}
	}()

	// Phase 2: playback
	func() {
		// Create a new recorder in replay mode
		rec2, err := recorder.New(cassPath, recorder.WithMode(recorder.ModeReplayOnly))
		if err != nil {
			t.Fatal(err)
		}
		defer rec2.Stop()

		if rec2.IsRecording() {
			t.Fatal("recorder should not be in recording mode for playback")
		}

		doRequest(rec2.GetDefaultClient())

		// Verify that the interaction was replayed (not re-recorded)
		c2, err := cassette.Load(cassPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(c2.Interactions) != 1 {
			t.Fatalf("Expected still 1 interaction after playback, got %d", len(c2.Interactions))
		}
	}()
}
