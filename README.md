## go-vcr

[![Build Status](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/gopkg.in/dnaeon/go-vcr.v4.svg)](https://pkg.go.dev/gopkg.in/dnaeon/go-vcr.v4)
[![Go Report Card](https://goreportcard.com/badge/gopkg.in/dnaeon/go-vcr.v4)](https://goreportcard.com/report/gopkg.in/dnaeon/go-vcr.v4)
[![codecov](https://codecov.io/gh/dnaeon/go-vcr/branch/v4/graph/badge.svg)](https://codecov.io/gh/dnaeon/go-vcr)

`go-vcr` simplifies testing by recording your HTTP interactions and replaying
them in future runs in order to provide fast, deterministic and accurate testing
of your code.

`go-vcr` was inspired by the [VCR library for Ruby](https://github.com/vcr/vcr).

## Installation

Install `go-vcr` by executing the command below:

```bash
$ go get -v gopkg.in/dnaeon/go-vcr.v4
```

Note, that if you are migrating from a previous version of `go-vcr`, you may
need to re-create your tests cassettes.

## Usage

A quick example of using `go-vcr`.

``` go
package helloworld_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func TestHelloWorld(t *testing.T) {
	// Create our recorder
	r, err := recorder.New(filepath.Join("testdata", strings.ReplaceAll(t.Name(), "/", "_")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Make sure recorder is stopped once done with it.
		if err := r.Stop(); err != nil {
			t.Error(err)
		}
	})

	client := r.GetDefaultClient()
	url := "https://go.dev/VERSION?m=text"

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get url %s: %s", url, err)
	}

	t.Logf("GET %s: %d\n", url, resp.StatusCode)
}
```

Running this test code for the first time will result in creating the
`testdata/TestHelloWorld.yaml` cassette, which will contain the recorded HTTP
interaction between our HTTP client and the remote server.

When we execute this test next time, what would happen is that `go-vcr` will
replay the already recorded HTTP interactions from the cassette, instead of
making actual external calls.

Please also check the [examples](./examples) directory from this repo for
complete and ready to run examples.

You can also refer to the [test cases](./pkg/recorder/recorder_test.go) for
additional examples.

## Custom Request Matching

During replay mode, you can customize the way incoming requests are matched
against the recorded request/response pairs by defining a `recorder.MatcherFunc`
function.

For example, the following matcher will match on method, URL and body:

``` go

func customMatcher(r *http.Request, i cassette.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return cassette.DefaultMatcher(r, i)
	}

	var reqBody []byte
	var err error
	reqBody, err = io.ReadAll(r.Body)
	if err != nil {
		log.Fatal("failed to read request body")
	}
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(reqBody))

	return r.Method == i.Method && r.URL.String() == i.URL && string(reqBody) == i.Body
}

...

// Recorder options
opts := []recorder.Option{
	recorder.WithCassette("testdata/matchers"),
	recorder.WithMatcher(customMatcher),
}

rec, err := recorder.New(opts...)
if err != nil {
        log.Fatal(err)
}
defer rec.Stop() // Make sure recorder is stopped once done with it

client := rec.GetDefaultClient()
resp, err := client.Get("https://www.google.com/")

...
```

## Hooks

Hooks in `go-vcr` are regular functions which take an HTTP interaction and are
invoked at different stages of the playback.

You can use hooks to modify a request/response before it is saved on disk,
before it is returned to the client, or anything else that you might want to do
with it, e.g. you might want to simply log each captured interaction.

You often provide sensitive data, such as API credentials, when making requests
against a service.

By default, this data will be stored in the recorded data but you probably don't
want this.

Removing or replacing data before it is stored can be done by adding one or more
`Hook`s to your `Recorder`.

There are different kinds of hooks, which are invoked in different stages of the
playback. The supported hook kinds are `AfterCaptureHook`, `BeforeSaveHook`,
`BeforeResponseReplayHook` and `OnRecorderStop`.

Here is an example that removes the `Authorization` header from all requests
right after capturing a new interaction.

``` go
// A hook which removes Authorization headers from all requests
hook := func(i *cassette.Interaction) error {
	delete(i.Request.Headers, "Authorization")
	return nil
}

// Recorder options
opts := []recorder.Option{
	recorder.WithCassette("testdata/filters"),
	recorder.WithHook(hook, recorder.AfterCaptureHook),
	recorder.WithMatcher(cassette.NewDefaultMatcher(cassette.WithIgnoreAuthorization())),
}

r, err := recorder.New(opts...)
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

...
```

Hooks added using `recorder.AfterCaptureHook` are applied right after an
interaction is captured and added to the in-memory cassette. This may not always
be what you need. For example if you modify an interaction using this hook kind
then subsequent test code will see the edited response.

For instance, if a response body contains an OAuth access token that is needed
for subsequent requests, then redacting the access token using a
`AfterCaptureHook` will result in authorization failures in subsequent test
code.

In such cases you would want to modify the recorded interactions right before
they are saved on disk. For that purpose you should be using a `BeforeSaveHook`,
e.g.

``` go
// Your test code will continue to see the real access token and
// it is redacted before the recorded interactions are saved on disk
hook := func(i *cassette.Interaction) error {
	if strings.Contains(i.Request.URL, "/oauth/token") {
		i.Response.Body = `{"access_token": "[REDACTED]"}`
	}

	return nil
}

// Recorder options
opts := []recorder.Option{
	recorder.WithCassette("testdata/filters"),
	recorder.WithHook(hook, recorder.BeforeSaveHook),
}

r, err := recorder.New(opts...)
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

...
```

## Passing Through Requests

Sometimes you want to allow specific requests to pass through to the remote
server without recording anything.

Globally, you can use `ModePassthrough` for this, but if you want to disable the
recorder for individual requests, you can add `Passthrough` handlers to the
recorder.

Here's an example to pass through requests to a specific endpoint:

``` go
passthrough := func(req *http.Request) bool {
	return req.URL.Path == "/login"
}

// Recorder options
opts := []recorder.Option{
	recorder.WithCassette("testdata/filters"),
	recorder.WithPassthrough(passthrough),
}

r, err := recorder.New(opts...)
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

...
```

## Server Side

VCR testing can also be used for creating server-side tests. Use the
`recorder.HTTPMiddleware` with an HTTP handler in order to create fixtures from
incoming requests and the handler's responses. Then, these requests can be
replayed and compared against the recorded responses to create a regression
test.

Rather than mocking/recording external HTTP interactions, this will record and
replay _incoming_ interactions with your application's HTTP server.

See [an example here](./examples/middleware_test.go).

## License

`go-vcr` is Open Source and licensed under the [BSD
License](http://opensource.org/licenses/BSD-2-Clause)
