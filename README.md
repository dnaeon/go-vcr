## go-vcr

[![Build Status](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/gopkg.in/dnaeon/go-vcr.v3.svg)](https://pkg.go.dev/gopkg.in/dnaeon/go-vcr.v3)
[![Go Report Card](https://goreportcard.com/badge/gopkg.in/dnaeon/go-vcr.v3)](https://goreportcard.com/report/gopkg.in/dnaeon/go-vcr.v3)
[![codecov](https://codecov.io/gh/dnaeon/go-vcr/branch/v3/graph/badge.svg)](https://codecov.io/gh/dnaeon/go-vcr)

`go-vcr` simplifies testing by recording your HTTP interactions and
replaying them in future runs in order to provide fast, deterministic
and accurate testing of your code.

`go-vcr` was inspired by the [VCR library for Ruby](https://github.com/vcr/vcr).

## Installation

Install `go-vcr` by executing the command below:

```bash
$ go get -v gopkg.in/dnaeon/go-vcr.v3/recorder
```

Note, that if you are migrating from a previous version of `go-vcr`,
you need re-create your test cassettes, because as of `go-vcr v3`
there is a new format of the cassette, which is not
backwards-compatible with older releases.

## Usage

Please check the [examples](./examples) from this repo for example
usage of `go-vcr`.

You can also refer to the [test cases](./recorder/recorder_test.go)
for additional examples.

## Custom Request Matching

During replay mode, You can customize the way incoming requests are
matched against the recorded request/response pairs by defining a
`MatcherFunc` function.

For example, the following matcher will match on method, URL and body:

```go
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

func recorderWithCustomMatcher() {
	rec, err := recorder.New("fixtures/matchers")
	if err != nil {
		log.Fatal(err)
	}
	defer rec.Stop() // Make sure recorder is stopped once done with it

	rec.SetReplayableInteractions(true)
	rec.SetMatcher(customMatcher)

	client := rec.GetDefaultClient()
	resp, err := client.Get("https://www.google.com/")
	...
	...
	...
}
```

## Hooks

Hooks in `go-vcr` are regular functions which take an HTTP interaction
and are invoked in different stages of the playback.

You can use hooks to modify a request/response before it is saved on
disk, before it is returned to the client, or anything else that you
might want to do with it, e.g. you might want to simply log each
captured interaction.

You often provide sensitive data, such as API credentials, when making
requests against a service.

By default, this data will be stored in the recorded data but you
probably don't want this.

Removing or replacing data before it is stored can be done by adding
one or more `Hook`s to your `Recorder`.

There are different kinds of hooks, which are invoked in different
stages of the playback. The supported hook kinds are
`AfterCaptureHook`, `BeforeSaveHook` and `BeforeResponseReplayHook`.

Here is an example that removes the `Authorization` header from all
requests right after capturing a new interaction.

```go
r, err := recorder.New("fixtures/filters")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

// Add a hook which removes Authorization headers from all requests
hook := func(i *cassette.Interaction) error {
	delete(i.Request.Headers, "Authorization")
	return nil
}
r.AddHook(hook, recorder.AfterCaptureHook)
```

Hooks added using `recorder.AfterCaptureHook` are applied right after
an interaction is captured and added to the in-memory cassette. This
may not always be what you need. For example if you modify an
interaction using this hook kind then subsequent test code will see
the edited response.

For instance, if a response body contains an OAuth access token that
is needed for subsequent requests, then redacting the access token
using a `AfterCaptureHook` will result in authorization failures in
subsequent test code.

In such cases you would want to modify the recorded interactions right
before they are saved on disk. For that purpose you should be using a
`BeforeSaveHook`, e.g.

```go
r, err := recorder.New("fixtures/filters")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

// Your test code will continue to see the real access token and
// it is redacted before the recorded interactions are saved on disk
hook := func(i *cassette.Interaction) error {
	if strings.Contains(i.Request.URL, "/oauth/token") {
		i.Response.Body = `{"access_token": "[REDACTED]"}`
	}

	return nil
}
r.AddHook(hook, recorder.BeforeSaveHook)
```

## Passing Through Requests

Sometimes you want to allow specific requests to pass through to the
remote server without recording anything.

Globally, you can use `ModePassthrough` for this, but if you want to
disable the recorder for individual requests, you can add
`Passthrough` handlers to the recorder.

The function takes a pointer to the original request, and returns a
boolean, indicating if the request should pass through to the remote
server.

Here's an example to pass through requests to a specific endpoint:

```go
// Passthrough the request to the remote server if the path matches "/login".
r.AddPassthrough(func(req *http.Request) bool {
    return req.URL.Path == "/login"
})
```

## License

`go-vcr` is Open Source and licensed under the
[BSD License](http://opensource.org/licenses/BSD-2-Clause)
