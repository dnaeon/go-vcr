package vcr

import (
	"net/http"
)

// Client request record type
type RequestRecord {
	// Body of request
	Body string `yaml:"body"`

	// Request headers
	Headers http.Header `yaml:"headers"`

	// Request URL
	URL string `yaml:"url"`

	// Request method
	Method string `yaml:"method"`
}

// Server response record type
type ResponseRecord struct {
	// Body of response
	Body string `yaml:"body"`

	// Response headers
	Headers http.Header `yaml:"headers"`

	// Response status message
	Status string `yaml:"status"`

	// Response status code
	Code int `yaml:"code"`
}

// CassetteRecord type represents an HTTP interaction between
// client and server. It contains the client request and the
// returned response from the server for this request.
type CassetteRecord struct {
	Request  RequestRecord  `yaml:"request"`
	Response ResponseRecord `yaml:"response"`
}
