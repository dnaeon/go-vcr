package vcr_test

import (
	"net/http"
	"net/http/httptest"
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
			w.Write([]byte("OK"))
		})

		if middleware != nil {
			handler = middleware(handler).ServeHTTP
		}

		mux.Handle("/", handler)
		return mux
	}

	t.Run("RecordRealInteractionsWithMiddleware", func(t *testing.T) {
		recorder, err := recorder.NewWithOptions(&recorder.Options{
			CassetteName: cassetteName,
			Mode:         recorder.ModeRecordOnly,
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
	})

	t.Run("ReplayCassetteAndCompare", func(t *testing.T) {
		cassette.TestServerReplay(t, cassetteName, createHandler(nil))
	})
}
