// Copyright (c) 2015 Marin Atanasov Nikolov <dnaeon@gmail.com>
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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/dnaeon/go-vcr/recorder"
)

type recordTest struct {
	method string
	body   io.Reader
	out    string
}

func (test recordTest) perform(t *testing.T, url string, r *recorder.Recorder) {
	// Create an HTTP client and inject our transport
	client := &http.Client{
		Transport: r.Transport, // Inject our transport!
	}

	req, err := http.NewRequest(test.method, url, test.body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != test.out {
		t.Fatalf("got:\t%s\n\twant:\t%s", string(content), string(test.out))
	}
}

func TestRecord(t *testing.T) {
	runID := time.Now().Format(time.RFC3339Nano)
	tests := []recordTest{
		{
			method: "GET",
			out:    "GET " + runID,
		},
		{
			method: "POST",
			body:   strings.NewReader("post body"),
			out:    "POST " + runID + "\npost body",
		},
	}

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	cassPath := path.Join(dir, "record_test")
	var serverURL string

	func() {
		// Start our recorder
		r, err := recorder.New(cassPath)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Stop() // Make sure recorder is stopped once done with it

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s %s", r.Method, runID)
			if r.Body != nil {
				defer r.Body.Close()
				fmt.Fprintln(w)
				io.Copy(w, r.Body)
			}
		}))
		defer server.Close()
		serverURL = server.URL

		for _, test := range tests {
			test.perform(t, serverURL, r)
		}
	}()

	// Re-run without the actual server
	func() {
		r, err := recorder.New(cassPath)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Stop()

		for _, test := range tests {
			test.perform(t, serverURL, r)
		}
	}()
}
