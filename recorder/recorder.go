package recorder

import (
	"io/ioutil"
	"os"
	"log"
	"net/http"
	"net/url"

	"github.com/dnaeon/vcr/cassette"
)

// Recorder states
const (
	ModeRecording = iota
	ModeReplaying
)

type Recorder struct {
	// Operating mode of the recorder
	mode int

	// HTTP server used to mock requests
	server *httptest.Server

	// Cassette used by the recorder
	cassette cassette.Cassette

 	// Proxy function that can be used by client transports
	ProxyFunc func(*http.Request) (*url.URL, error)

	// Default transport that can be used by clients to inject
	Transport *http.Transport
}

// Proxies client requests to their original destination
func requestHandler(r *http.Request, c *cassette.Cassette, mode int) (*cassette.Interaction, error) {
	// Return interaction from cassette if in replay mode
	if mode == ModeReplaying {
		return c.Get(r)
	}

	// Else, perform client request to their original
	// destination and record interactions
	client := &http.Client{}
	req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		return nil, err
	}

	req.Header = r.Header
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Record the interaction and add it to the cassette
	reqBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Add interaction to cassette
	interaction := cassette.Interaction{
		Request: cassette.Request{
			Body:    string(reqBody),
			Headers: req.Header,
			URL:     req.URL.String(),
			Method:  req.Method,
		},
		Response: cassette.Response{
			Body:    string(respBody),
			Headers: resp.Header,
			Status:  resp.Status,
			Code:    resp.StatusCode,
		},
	}
	c.Add(interaction)

	return interaction, nil
}

// Creates a new recorder
func NewRecorder(cassetteName string) *Recorder {
	c := cassette.NewCassette(cassetteName)
	if os.Stat(cassetteName); os.IsNotExist(err) {
		mode := ModeRecording
	} else {
		mode := ModeReplaying
	}

	// Handler for client requests
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pass cassette to handler for recording and replaying of interactions
		interaction, err := requestHandler(r, c, mode)
		if err != nil {
			log.Fatal(err)
		}

		w.WriteHeader(interaction.Response.Code)
		fmt.Fprintln(w, interaction.Response.Body)
	})

	// HTTP server used to mock requests
	server := httptest.NewUnstartedServer(handler)

	// A proxy function which routes all requests through our HTTP server
	// Can be used by clients to inject into their own transports
	proxyFunc := func(*http.Request) (*url.URL, error) {
		return url.Parse(server.URL)
	}

	// A transport which can be used by clients to inject
	transport := &http.Transport{
		Proxy: proxyFunc,
	}

	r := &Recorder{
		mode:      mode,
		server:    server,
		cassette:  c,
		ProxyFunc: proxyFunc,
		Transport: transport,
	}

	return r
}

// Starts the recorder
func (r *Recorder) Start() error {
	// Load cassette data if in replay mode
	if r.mode == ModeReplaying {
		if err := r.cassette.Load(); err != nil {
			return err
		}
	}

	// Start HTTP server to mock request
	r.server.Start()

	return nil
}

// Stops the recorder
func (r *Recorder) Stop() error {
	r.server.Close()

	// Save cassette if in recording mode
	if r.mode == ModeRecording {
		if err := r.cassette.Save(); err != nil {
			return err
		}
	}

	return nil
}
