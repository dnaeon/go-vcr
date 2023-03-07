// Copyright (c) 2016-2022 Marin Atanasov Nikolov <dnaeon@gmail.com>
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

package vcr_test

import (
	"embed"
	"io/ioutil"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

//go:embed fixtures
var f embed.FS

func TestHTTPS(t *testing.T) {
	// Start our recorder
	r, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName: "fixtures/iana-reserved-domains",
		Mode:         recorder.ModeRecordOnce,
		Fs:           f,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop() // Make sure recorder is stopped once done with it

	if r.Mode() != recorder.ModeRecordOnce {
		t.Fatal("Recorder should be in ModeRecordOnce")
	}

	client := r.GetDefaultClient()
	url := "https://www.iana.org/domains/reserved"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get url %s: %s", url, err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %s", err)
	}

	wantHeading := "<h1>IANA-managed Reserved Domains</h1>"
	bodyContent := string(body)

	if !strings.Contains(bodyContent, wantHeading) {
		t.Errorf("Heading %s not found in response", wantHeading)
	}

	// This one should fail, because the recorder is in
	// ModeRecordOnce mode, and there is no recorded interaction
	// for this URL During first-time recording this block is
	// usually commented out, so it doesn't end up in the initial
	// recording.
	resp, err = client.Get("https://www.google.com/")
	if err == nil {
		t.Fatal("Request to www.google.com has succeeded, and it should fail")
	}
}
