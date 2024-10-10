// Copyright (c) 2015-2024 Marin Atanasov Nikolov <dnaeon@gmail.com>
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

package cassette

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func getMatcherRequests(t *testing.T) (*http.Request, Request) {
	method := "POST"
	host := "example.com"
	path := "/"
	urlStr := fmt.Sprintf("http://%s%s", host, path)
	u, err := url.Parse(urlStr)
	if err != nil {
		t.Fatal(err)
	}
	protoMajor := 1
	protoMinor := 0
	proto := fmt.Sprintf("HTTP/%d.%d", protoMajor, protoMinor)
	header := http.Header{
		"foo": {"1", "2"},
		"bar": {"3", "4"},
	}
	bodyStr := "foo\nbar"
	hReqBody := io.NopCloser(strings.NewReader(bodyStr))
	contentLength := int64(len(bodyStr))
	transferEncoding := []string{"bar", "baz"}
	form := url.Values{}
	form.Set("name", "Ava")
	form.Add("friend", "Jess")
	trailer := http.Header{
		"baz": {"5", "6"},
		"meh": {"7", "8"},
	}
	remoteAddr := "1.2.3.4"
	requestURI := fmt.Sprintf("%s %s %s", method, path, proto)

	r := &http.Request{
		Method:           "POST",
		URL:              u,
		Proto:            proto,
		ProtoMajor:       protoMajor,
		ProtoMinor:       protoMinor,
		Header:           header,
		Body:             hReqBody,
		ContentLength:    contentLength,
		TransferEncoding: transferEncoding,
		Host:             host,
		Form:             form,
		Trailer:          trailer,
		RemoteAddr:       remoteAddr,
		RequestURI:       requestURI,
	}

	i := Request{
		Proto:            proto,
		ProtoMajor:       protoMajor,
		ProtoMinor:       protoMinor,
		ContentLength:    contentLength,
		TransferEncoding: transferEncoding,
		Trailer:          trailer,
		Host:             host,
		RemoteAddr:       remoteAddr,
		RequestURI:       requestURI,
		Body:             bodyStr,
		Form:             form,
		Headers:          header,
		URL:              urlStr,
		Method:           "POST",
	}

	return r, i
}

func TestMatcher(t *testing.T) {
	t.Run("nil options", func(t *testing.T) {
		matcherFn := DefaultMatcher

		t.Run("match", func(t *testing.T) {
			r, i := getMatcherRequests(t)

			if b := matcherFn(r, i); !b {
				t.Fatalf("request should have matched")
			}
		})

		t.Run("not match Proto", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Proto = "foo"
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match ProtoMajor", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.ProtoMajor = 3
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match ProtoMinor", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.ProtoMinor = 5
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match ContentLength", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.ContentLength = r.ContentLength / 2
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match TransferEncoding", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.TransferEncoding = []string{"no", "match"}
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Trailer", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Trailer = http.Header{
				"not": {"a", "match"},
			}
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Host", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Host = "not.match"
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match RemoteAddr", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.RemoteAddr = "6.6.6.6"
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match RequestURI", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.RequestURI = "GET /not-match HTTP/1.0"
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Body", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Body = io.NopCloser(strings.NewReader("not a match"))
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Form", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Form = url.Values{"not": {"a", "match"}}
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Headers", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Header = http.Header{"not": {"a", "match"}}
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match URL", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			u, err := url.Parse("http://not.match/")
			if err != nil {
				t.Fatal(err)
			}
			r.URL = u
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})

		t.Run("not match Method", func(t *testing.T) {
			r, i := getMatcherRequests(t)
			r.Method = "DELETE"
			if b := matcherFn(r, i); b {
				t.Fatalf("request should not have matched")
			}
		})
	})

	t.Run("IgnoreUserAgent", func(t *testing.T) {
		matcherFn := NewDefaultMatcher(WithIgnoreUserAgent())

		t.Run("match", func(t *testing.T) {
			r, i := getMatcherRequests(t)

			r.Header = http.Header{
				"User-Agent": {"foo", "bar"},
			}

			i.Headers = http.Header{
				"User-Agent": {"baz", "meh"},
			}

			if b := matcherFn(r, i); !b {
				t.Fatalf("request should have matched")
			}
		})
	})

	t.Run("IgnoreAuthorization", func(t *testing.T) {
		matcherFn := NewDefaultMatcher(WithIgnoreAuthorization())

		t.Run("match", func(t *testing.T) {
			r, i := getMatcherRequests(t)

			r.Header = http.Header{
				"Authorization": {"Bearer xyz"},
			}

			i.Headers = http.Header{}

			if b := matcherFn(r, i); !b {
				t.Fatalf("request should have matched")
			}
		})
	})

	t.Run("IgnoreHeaders", func(t *testing.T) {
		matcherFn := NewDefaultMatcher(WithIgnoreHeaders("Header-One", "Header-Two"), WithIgnoreUserAgent(), WithIgnoreAuthorization())

		t.Run("match", func(t *testing.T) {
			r, i := getMatcherRequests(t)

			r.Header = http.Header{
				"Header-One": {"foo"},
				"Header-Two": {"foo"},
				"User-Agent": {"foo", "bar"},
			}

			i.Headers = http.Header{
				"Header-One":    {"bar"},
				"Header-Two":    {"bar"},
				"Authorization": {"Bearer xyz"},
			}

			if b := matcherFn(r, i); !b {
				t.Fatalf("request should have matched")
			}
		})
	})
}
