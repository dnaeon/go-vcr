package cassette

import (
	"os"
	"net/http"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Client request type
type request {
	// Body of request
	Body string `yaml:"body"`

	// Request headers
	Headers http.Header `yaml:"headers"`

	// Request URL
	URL string `yaml:"url"`

	// Request method
	Method string `yaml:"method"`
}

// Server response type
type response struct {
	// Body of response
	Body string `yaml:"body"`

	// Response headers
	Headers http.Header `yaml:"headers"`

	// Response status message
	Status string `yaml:"status"`

	// Response status code
	Code int `yaml:"code"`
}

// Interaction type contains a pair of request/response for a
// single HTTP interaction between a client and a server 
type Interaction struct {
	Request  request  `yaml:"request"`
	Response response `yaml:"response"`
}

// Cassette type
type Cassette struct {
	// Name of the cassette file
	Name string `yaml:"name"`

	// Cassette format version
	Version int `yaml:"version"`

	// Interactions between client and server
	Interactions []Interaction `yaml:"interactions"`
}
