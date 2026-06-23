// Copyright (c) 2015-2026 Marin Atanasov Nikolov <dnaeon@gmail.com>
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
	"bytes"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// TestDebugTrace demonstrates how to enable the recorder's debug trace using
// the WithDebugWriter option. The trace is a structured log of recorder
// lifecycle events.
//
// In a real test you would typically pass os.Stderr to WithDebugWriter, or set
// the VCR_DEBUG environment variable to a value parseable as true by
// strconv.ParseBool to enable the same trace on os.Stderr without changing the
// code:
//
//	VCR_DEBUG=true go test ./...
//
// Here we capture the trace into a bytes.Buffer so the example is deterministic
// and can assert that the expected event names appear in the output.
func TestDebugTrace(t *testing.T) {
	var traceBuf bytes.Buffer

	r, err := recorder.New(
		"testdata/debug-trace",
		recorder.WithDebugWriter(&traceBuf),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Make sure recorder is stopped once done with it.
		if err := r.Stop(); err != nil {
			t.Error(err)
		}
	})

	client := r.GetDefaultClient()
	url := "https://go.dev/VERSION?m=text"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get url %s: %s", url, err)
	}
	resp.Body.Close()

	// The captured trace should include the recorder's lifecycle events and
	// the per-request breadcrumbs for the request we just made. Each event
	// is one line in the trace and carries structured key=value attributes.
	trace := traceBuf.String()
	for _, want := range []string{
		"recorder initialized",
		"request received",
	} {
		if !strings.Contains(trace, want) {
			t.Errorf("debug trace missing %q\n--- trace ---\n%s", want, trace)
		}
	}
}
