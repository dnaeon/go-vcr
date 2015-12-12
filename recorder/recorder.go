package recorder

import (
	"os"
	"log"

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

	// Proxy function that can be used by client transports
	proxyFunc    func(*http.Request) (*url.URL, error)

	// Default transport that can be used by clients to inject
	transport *http.Transport

	// Cassette used by the recorder
	cassette cassette.Cassette
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
		// Pass cassette to handler for recording and replaying interactions
		interaction, err := proxyRequestHandler(r, c)
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
		proxyFunc: proxyFunc,
		transport: transport,
		cassette:  c,
	}

	return r
}

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
