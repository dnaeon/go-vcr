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

func TestIsPrintable(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if !isPrintable(nil) {
			t.Fatalf("nil should be printable")
		}
		if !isPrintable([]byte{}) {
			t.Fatalf("empty slice should be printable")
		}
	})

	t.Run("plain ASCII", func(t *testing.T) {
		if !isPrintable([]byte("hello world")) {
			t.Fatalf("plain ASCII should be printable")
		}
	})

	t.Run("HTTP whitespace runes are allowed", func(t *testing.T) {
		if !isPrintable([]byte("a\tb\nc\rd")) {
			t.Fatalf("tab, LF and CR should be allowed")
		}
	})

	t.Run("NUL is not printable", func(t *testing.T) {
		if isPrintable([]byte{'a', 0x00, 'b'}) {
			t.Fatalf("NUL byte should not be printable")
		}
	})

	t.Run("other control chars are not printable", func(t *testing.T) {
		if isPrintable([]byte{'a', 0x01, 'b'}) {
			t.Fatalf("0x01 should not be printable")
		}
		if isPrintable([]byte{'a', 0x1F, 'b'}) {
			t.Fatalf("0x1F should not be printable")
		}
	})

	t.Run("invalid UTF-8 is not printable", func(t *testing.T) {
		if isPrintable([]byte{0xFF, 0xFE, 0xFD}) {
			t.Fatalf("invalid UTF-8 should not be printable")
		}
	})

	t.Run("valid multi-byte UTF-8 is printable", func(t *testing.T) {
		if !isPrintable([]byte("héllo wörld")) {
			t.Fatalf("valid multi-byte UTF-8 should be printable")
		}
	})
}

func TestFormatBody(t *testing.T) {
	t.Run("empty body renders as empty string", func(t *testing.T) {
		if got := formatBody(nil); got != "" {
			t.Fatalf("got %q, want empty string", got)
		}
		if got := formatBody([]byte{}); got != "" {
			t.Fatalf("got %q, want empty string", got)
		}
	})

	t.Run("plain text is rendered verbatim", func(t *testing.T) {
		in := []byte("hello world")
		if got := formatBody(in); got != "hello world" {
			t.Fatalf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("binary body renders as placeholder", func(t *testing.T) {
		in := []byte{0x00, 0x01, 0x02, 0x03}
		want := fmt.Sprintf("<binary, %d bytes>", len(in))
		if got := formatBody(in); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("body larger than debugBodyLimit is truncated", func(t *testing.T) {
		in := make([]byte, debugBodyLimit+128)
		for i := range in {
			in[i] = 'a'
		}
		got := formatBody(in)
		if !strings.HasPrefix(got, strings.Repeat("a", debugBodyLimit)) {
			t.Fatalf("truncated body does not start with the first debugBodyLimit bytes")
		}
		wantSuffix := fmt.Sprintf("(truncated, %d bytes total)", len(in))
		if !strings.HasSuffix(got, wantSuffix) {
			t.Fatalf("got %q, want suffix %q", got, wantSuffix)
		}
	})
}

func TestSummarizeBody(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		if got := summarizeBody(""); got != "" {
			t.Fatalf("got %q, want empty string", got)
		}
	})

	t.Run("plain body is returned unchanged", func(t *testing.T) {
		if got := summarizeBody("hello"); got != "hello" {
			t.Fatalf("got %q, want %q", got, "hello")
		}
	})

	t.Run("newlines are escaped", func(t *testing.T) {
		got := summarizeBody("foo\nbar\rbaz")
		want := `foo\nbar\rbaz`
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("oversized body is truncated", func(t *testing.T) {
		in := strings.Repeat("a", debugSummaryBodyLimit+10)
		got := summarizeBody(in)
		want := strings.Repeat("a", debugSummaryBodyLimit) + "..."
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestSummarizeCassetteRequest(t *testing.T) {
	req := Request{
		Method: "GET",
		URL:    "https://example.com/foo",
		Body:   "",
	}
	want := `GET https://example.com/foo body=""`
	if got := summarizeCassetteRequest(req); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSummarizeCassetteResponse(t *testing.T) {
	resp := Response{
		Code: 200,
		Body: "ok",
	}
	want := `200 body="ok"`
	if got := summarizeCassetteResponse(resp); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDumpHTTPRequestRestoresBody(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
		if err != nil {
			t.Fatal(err)
		}
		got := dumpHTTPRequest(r)
		if !strings.Contains(got, "GET / HTTP/1.1") {
			t.Fatalf("dump missing request line: %q", got)
		}
	})

	t.Run("body is restored and dump contains it", func(t *testing.T) {
		body := "hello world"
		r, err := http.NewRequest(http.MethodPost, "http://example.com/", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		got := dumpHTTPRequest(r)
		if !strings.Contains(got, body) {
			t.Fatalf("dump %q does not contain body %q", got, body)
		}
		// The body must still be readable by subsequent code (the matcher,
		// the round-tripper, etc.).
		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("body unreadable after dump: %v", err)
		}
		if string(read) != body {
			t.Fatalf("got body %q after dump, want %q", string(read), body)
		}
	})
}

func TestDumpCassetteRequest(t *testing.T) {
	req := Request{
		Method:  "POST",
		URL:     "https://example.com/v1",
		Proto:   "HTTP/1.1",
		Host:    "example.com",
		Headers: http.Header{"Content-Type": {"application/json"}},
		Body:    `{"x":1}`,
	}
	got := dumpCassetteRequest(req)
	if !strings.Contains(got, "POST https://example.com/v1 HTTP/1.1") {
		t.Fatalf("dump missing request line: %q", got)
	}
	if !strings.Contains(got, "Host: example.com") {
		t.Fatalf("dump missing Host header: %q", got)
	}
	if !strings.Contains(got, "Content-Type: application/json") {
		t.Fatalf("dump missing Content-Type header: %q", got)
	}
	if !strings.Contains(got, `{"x":1}`) {
		t.Fatalf("dump missing body: %q", got)
	}
}
