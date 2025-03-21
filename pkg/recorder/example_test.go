// Copyright (c) 2016-2024 Marin Atanasov Nikolov <dnaeon@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//  1. Redistributions of source code must retain the above copyright
//     notice, this list of conditions and the following disclaimer
//     in this position and unchanged.
//  2. Redistributions in binary form must reproduce the above copyright
//     notice, this list of conditions and the following disclaimer in the
//     documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE AUTHOR(S) “AS IS” AND ANY EXPRESS OR
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
	"log"
	"path/filepath"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func ExampleNew() {
	// Create our recorder
	r, err := recorder.New(filepath.Join("testdata", "hello-world"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		// Make sure recorder is stopped once done with it.
		if err := r.Stop(); err != nil {
			log.Fatal(err)
		}
	}()

	client := r.GetDefaultClient()
	url := "https://go.dev/VERSION?m=text"

	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("Failed to get url %s: %s", url, err)
	}

	fmt.Printf("GET %s: %d\n", url, resp.StatusCode)
	// Output:
	// GET https://go.dev/VERSION?m=text: 200
}
