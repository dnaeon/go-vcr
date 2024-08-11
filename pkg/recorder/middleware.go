package recorder

import (
	"net/http"
	"net/http/httptest"
)

// Middleware intercepts and records all incoming requests and the server's response
func (rec *Recorder) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := newPassthrough(w)
		next.ServeHTTP(ww, r)

		// On the server side, requests do not have Host and Scheme so it must be set
		r.URL.Host = "go-vcr"
		r.URL.Scheme = "http"

		// copy headers from real response
		for k, vv := range ww.real.Header() {
			for _, v := range vv {
				ww.recorder.Result().Header.Add(k, v)
			}
		}

		_, _ = rec.executeAndRecord(r, ww.recorder.Result())
	})
}

var _ http.ResponseWriter = &passthroughWriter{}

// passthroughWriter uses the original ResponseWriter and an httptest.ResponseRecorder
// so the middleware can capture response details and passthrough to the client
type passthroughWriter struct {
	recorder *httptest.ResponseRecorder
	real     http.ResponseWriter
}

func newPassthrough(real http.ResponseWriter) passthroughWriter {
	return passthroughWriter{recorder: httptest.NewRecorder(), real: real}
}

func (p passthroughWriter) Header() http.Header {
	return p.real.Header()
}

func (p passthroughWriter) Write(in []byte) (int, error) {
	_, _ = p.recorder.Write(in)
	return p.real.Write(in)
}

func (p passthroughWriter) WriteHeader(statusCode int) {
	p.recorder.WriteHeader(statusCode)
	p.real.WriteHeader(statusCode)
}
