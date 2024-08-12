package cassette

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// TestServerReplay loads a Cassette and replays each Interaction with the provided Handler, then compares the response
func TestServerReplay(t *testing.T, cassetteName string, handler http.Handler) {
	t.Helper()

	c, err := Load(cassetteName)
	if err != nil {
		t.Errorf("unexpected error loading Cassette: %v", err)
	}

	if len(c.Interactions) == 0 {
		t.Error("no interactions in Cassette")
	}

	for _, interaction := range c.Interactions {
		t.Run(fmt.Sprintf("Interaction_%d", interaction.ID), func(t *testing.T) {
			TestInteractionReplay(t, handler, interaction)
		})
	}
}

// TestInteractionReplay replays an Interaction with the provided Handler and compares the response
func TestInteractionReplay(t *testing.T, handler http.Handler, interaction *Interaction) {
	t.Helper()

	req, err := interaction.GetHTTPRequest()
	if err != nil {
		t.Errorf("unexpected error getting interaction request: %v", err)
	}

	if len(req.Form) > 0 {
		req.Body = io.NopCloser(strings.NewReader(req.Form.Encode()))
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	expectedResp, err := interaction.GetHTTPResponse()
	if err != nil {
		t.Errorf("unexpected error getting interaction response: %v", err)
	}

	if expectedResp.StatusCode != w.Result().StatusCode {
		t.Errorf("status code does not match: expected=%d actual=%d", expectedResp.StatusCode, w.Result().StatusCode)
	}

	expectedBody, err := io.ReadAll(expectedResp.Body)
	if err != nil {
		t.Errorf("unexpected reading response body: %v", err)
	}

	if string(expectedBody) != w.Body.String() {
		t.Errorf("body does not match: expected=%s actual=%s", expectedBody, w.Body.String())
	}

	if !headersEqual(expectedResp.Header, w.Header()) {
		t.Errorf("header values do not match. expected=%v actual=%v", expectedResp.Header, w.Header())
	}
}

func headersEqual(expected, actual http.Header) bool {
	return maps.EqualFunc(
		expected, actual,
		func(v1, v2 []string) bool {
			slices.Sort(v1)
			slices.Sort(v2)

			if !slices.Equal(v1, v2) {
				return false
			}

			return true
		},
	)
}
