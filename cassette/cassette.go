package cassette

import (
	"io/ioutil"
	"os"
	"net/http"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Cassette format versions
const (
	cassetteFormatV1 = 1
)

var (
	RequestNotFound = errors.New("No matching request found in cassette")
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
	Name string `yaml:"-"`

	// Cassette format version
	Version int `yaml:"version"`

	// Interactions between client and server
	Interactions []Interaction `yaml:"interactions"`
}

// Creates a new cassette
func NewCassette(name string) *Cassette {
	c := &Cassette{
		Name:         name,
		Version:      cassetteFormatV1,
		Interactions: make([]Interaction, 0),
	}

	return c
}

// Adds a new interaction to the cassette
func (c *Cassette) Add(i *Interaction) {
	c.Interactions = append(c.Interactions, i)
}

// Gets a recorded interaction
func (c *Cassette) Get(r *http.Request) (*Interaction, error) {
	for _, i := range c.Interactions {
		if r.Method == i.Request.Method && r.URL == i.Request.URL {
			return i, nil
		}
	}

	return nil, RequestNotFound
}

// Loads a cassette from file
func (c *Cassette) Load() error {
	cassetteFile := filepath.Join(c.Name, ".yaml")

	data, err := ioutil.ReadFile(cassetteFile)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &c)

	return err
}

// Saves the cassette on disk for future use
func (c *Cassette) Save() error {
	cassetteFile := filepath.Join(c.Name, ".yaml")
	cassetteDir := filepath.Dir(cassetteFile)

	// Create directory for cassette if missing
	if _, err := os.Stat(cassetteDir); os.IsNotExist(err) {
		if err = os.MkdirAll; err != nil {
			return err
		}
	}

	// Marshal to YAML and save interactions
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(cassetteFile, data, 0644)

	return err
}
