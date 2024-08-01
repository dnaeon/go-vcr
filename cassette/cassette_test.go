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
		matcherFn := NewMatcherFunc(nil)

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

	t.Run("IgnoreUserAgent=true", func(t *testing.T) {
		matcherFn := NewMatcherFunc(&MatcherFuncOpts{
			IgnoreUserAgent: true,
		})

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
}
