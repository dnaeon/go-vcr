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

	"bytes"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
)

type recordTest struct {
	method         string
	body           string
	want           string
	wantContentLen int
}

func httpTests(runID string) []recordTest {
	return []recordTest{
		{
			method:         "GET",
			want:           "GET " + runID + "\n",
			wantContentLen: 4 + len(runID) + 1,
		},
		{
			method:         "HEAD",
			wantContentLen: 5 + len(runID) + 1,
		},
		{
			method:         "POST",
			body:           "post body",
			want:           "POST " + runID + "\npost body",
			wantContentLen: 5 + len(runID) + 10,
		},
		{
			method:         "POST",
			body:           "alt body",
			want:           "POST " + runID + "\nalt body",
			wantContentLen: 5 + len(runID) + 9,
		},
	}
}

func TestRecord(t *testing.T) {
	runID, cassPath, tests := setupTests(t, "record_test")

	r, serverURL := httpRecorderTest(t, runID, tests, cassPath, recorder.ModeRecording)

	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	if m := r.Mode(); m != recorder.ModeRecording {
		t.Fatalf("Expected recording mode, got %v", m)
	}

	for i, test := range tests {
		body := c.Interactions[i].Request.Body
		if body != test.body {
			t.Fatalf("got:\t%s\n\twant:\t%s", string(body), string(test.body))
		}
	}

	// Re-run without the actual server
	r, err = recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	if m := r.Mode(); m != recorder.ModeReplaying {
		t.Fatalf("Expected replaying mode, got %v", m)
	}

	// Use a custom matcher that includes matching on request body
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		var b bytes.Buffer
		if _, err := b.ReadFrom(r.Body); err != nil {
			t.Fatalf("unable to read request body: %s", err)
			return false
		}
		r.Body = ioutil.NopCloser(&b)
		return cassette.DefaultMatcher(r, i) && (b.String() == "" || b.String() == i.Body)
	})

	t.Log("replaying")
	for _, test := range tests {
		test.perform(t, serverURL, r)
	}
}

func TestModeContextTimeout(t *testing.T) {
	// Record initial requests
	runID, cassPath, tests := setupTests(t, "record_playback_timeout")
	_, serverURL := httpRecorderTest(t, runID, tests, cassPath, recorder.ModeReplaying)

	// Re-run without the actual server
	r, err := recorder.New(cassPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		cancelFn()
		_, err := test.performReq(t, ctx, serverURL, r)
		if err == nil || err == cassette.ErrInteractionNotFound {
			t.Fatalf("Expected cancellation error, got %v", err)
		}
	}
}

func TestModePlaybackMissing(t *testing.T) {
	// Record initial requests
	runID, cassPath, tests := setupTests(t, "record_playback_missing_test")
	httpRecorderTest(t, runID, tests, cassPath, recorder.ModeReplaying)

	// setup same path but a new runID so requests won't match
	runID = time.Now().Format(time.RFC3339Nano)
	recorder, server := httpRecorderTestSetup(t, runID, cassPath, recorder.ModeReplaying)
	serverURL := server.URL

	defer server.Close()
	defer recorder.Stop()

	for _, test := range tests {
		resp, err := test.performReq(t, context.Background(), serverURL, recorder)
		if resp != nil {
			t.Fatalf("Expected response to be nil but was %v", resp)
		}

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
	runID, cassPath, tests := setupTests(t, "record_disabled_test")

	r, _ := httpRecorderTest(t, runID, tests, cassPath, recorder.ModeDisabled)

	if m := r.Mode(); m != recorder.ModeDisabled {
		t.Fatalf("Expected disabled mode, got %v", m)
	}

	_, err := cassette.Load(cassPath)
	// Expect the file to not exist if record is disabled
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPassthrough(t *testing.T) {
	runID, cassPath, tests := setupTests(t, "test_passthrough")
	recorder, server := httpRecorderTestSetup(t, runID, cassPath, recorder.ModeRecording)
	serverURL := server.URL

	// Add a passthrough configuration which does not record any requests with
	// a specific body.
	recorder.AddPassthrough(func(r *http.Request) bool {
		if r.Body == nil {
			return false
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(r.Body); err != nil {
			return false
		}
		r.Body = ioutil.NopCloser(&b)

		return b.String() == "alt body"
	})

	t.Log("make http requests")
	for _, test := range tests {
		test.perform(t, serverURL, recorder)
	}

	// Make sure recorder is stopped once done with it
	server.Close()
	t.Log("server shut down")

	recorder.Stop()
	t.Log("recorder stopped")

	// Load the cassette we just stored:
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that no body exists matching our pass through test
	for _, i := range c.Interactions {
		body := i.Request.Body
		if body == "alt body" {
			t.Fatalf("unexpected recording:\t%s", body)
		}
	}
}

func TestFilter(t *testing.T) {
	dummyBody := "[REDACTED]"

	runID, cassPath, tests := setupTests(t, "test_filter")
	recorder, server := httpRecorderTestSetup(t, runID, cassPath, recorder.ModeRecording)
	serverURL := server.URL

	// Add a filter which replaces each request body in the stored cassette:
	recorder.AddFilter(func(i *cassette.Interaction) error {
		i.Request.Body = dummyBody
		return nil
	})

	t.Log("make http requests")
	for _, test := range tests {
		test.perform(t, serverURL, recorder)
	}

	// Make sure recorder is stopped once done with it
	server.Close()
	t.Log("server shut down")

	recorder.Stop()
	t.Log("recorder stopped")

	// Load the cassette we just stored:
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that each body has been set to our dummy value
	for i := range tests {
		body := c.Interactions[i].Request.Body
		if body != dummyBody {
			t.Fatalf("got:\t%s\n\twant:\t%s", string(body), string(dummyBody))
		}
	}
}

func TestSaveFilter(t *testing.T) {
	dummyBody := "[REDACTED]"

	runID, cassPath, tests := setupTests(t, "test_save_filter")
	recorder, server := httpRecorderTestSetup(t, runID, cassPath, recorder.ModeRecording)
	serverURL := server.URL

	// Add a filter which replaces each request body in the stored cassette:
	recorder.AddSaveFilter(func(i *cassette.Interaction) error {
		i.Response.Body = dummyBody
		return nil
	})

	t.Log("make http requests")
	for _, test := range tests {
		test.perform(t, serverURL, recorder)
	}

	// Make sure recorder is stopped once done with it
	server.Close()
	t.Log("server shut down")

	recorder.Stop()
	t.Log("recorder stopped")

	// Load the cassette we just stored:
	c, err := cassette.Load(cassPath)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that each body has been set to our dummy value
	for i := range tests {
		body := c.Interactions[i].Response.Body
		if body != dummyBody {
			t.Fatalf("got:\t%s\n\twant:\t%s", string(body), string(dummyBody))
		}
	}
}

func httpRecorderTestSetup(t *testing.T, runID string, cassPath string, mode recorder.Mode) (*recorder.Recorder, *httptest.Server) {
	// Start our recorder
	recorder, err := recorder.NewAsMode(cassPath, mode, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", r.Method, runID)
		if r.Body != nil {
			defer r.Body.Close()
			fmt.Fprintln(w)
			io.Copy(w, r.Body)
		}
	}))

	return recorder, server
}

func httpRecorderTest(t *testing.T, runID string, tests []recordTest, cassPath string, mode recorder.Mode) (*recorder.Recorder, string) {
	recorder, server := httpRecorderTestSetup(t, runID, cassPath, mode)
	serverURL := server.URL

	t.Log("test http requests")
	for _, test := range tests {
		test.perform(t, serverURL, recorder)
	}

	// Make sure recorder is stopped once done with it
	server.Close()
	t.Log("server shut down")

	recorder.Stop()
	t.Log("recorder stopped")

	return recorder, serverURL
}

func (test recordTest) perform(t *testing.T, url string, r *recorder.Recorder) {
	resp, err := test.performReq(t, context.Background(), url, r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != test.want {
		t.Fatalf("got:\t%s\n\twant:\t%s", string(content), test.want)
	}
	if resp.ContentLength != int64(test.wantContentLen) {
		t.Fatalf("got ContentLength %d want %d", resp.ContentLength, test.wantContentLen)
	}
}

func (test recordTest) performReq(t *testing.T, ctx context.Context, url string, r *recorder.Recorder) (*http.Response, error) {
	// Create an HTTP client and inject our transport
	client := &http.Client{
		Transport: r, // Inject as transport!
	}

	req, err := http.NewRequest(test.method, url, strings.NewReader(test.body))
	if err != nil {
		t.Fatal(err)
	}
	return client.Do(req.WithContext(ctx))
}

func setupTests(t *testing.T, name string) (runID, cassPath string, tests []recordTest) {
	runID = time.Now().Format(time.RFC3339Nano)

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	cassPath = path.Join(dir, name)
	tests = httpTests(runID)

	return
}
