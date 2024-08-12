package vcr_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v3/cassette"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

func TestMiddleware(t *testing.T) {
	cassetteName := "fixtures/middleware"
	createHandler := func(middleware func(http.Handler) http.Handler) http.Handler {
		mux := http.NewServeMux()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("KEY", "VALUE")

			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				w.Write(body)
			} else {
				w.Write([]byte("OK"))
			}
		})

		if middleware != nil {
			handler = middleware(handler).ServeHTTP
		}

		mux.Handle("/", handler)
		return mux
	}

	// In a real-world scenario, the recorder will run outside of unit tests
	// since you want to be able to record real application behavior
	t.Run("RecordRealInteractionsWithMiddleware", func(t *testing.T) {
		recorder, err := recorder.NewWithOptions(&recorder.Options{
			CassetteName:                    cassetteName,
			Mode:                            recorder.ModeRecordOnly,
			BlockRealTransportUnsafeMethods: false,
		})
		if err != nil {
			t.Errorf("error creating recorder: %v", err)
		}

		// Create the server handler with recorder middleware
		handler := createHandler(recorder.Middleware)
		defer recorder.Stop()

		server := httptest.NewServer(handler)
		defer server.Close()

		_, err = http.Get(server.URL + "/request1")
		if err != nil {
			t.Errorf("error making request: %v", err)
		}

		_, err = http.Get(server.URL + "/request2")
		if err != nil {
			t.Errorf("error making request: %v", err)
		}

		_, err = http.PostForm(server.URL+"/postform", url.Values{"key": []string{"value"}})
		if err != nil {
			t.Errorf("error making request: %v", err)
		}

		_, err = http.Post(server.URL+"/postdata", "application/json", bytes.NewBufferString(`{"key":"value"}`))
		if err != nil {
			t.Errorf("error making request: %v", err)
		}
	})

	t.Run("ReplayCassetteAndCompare", func(t *testing.T) {
		cassette.TestServerReplay(t, cassetteName, createHandler(nil))
	})
}
