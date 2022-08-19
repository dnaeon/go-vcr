## go-vcr

[![Build Status](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)](https://github.com/dnaeon/go-vcr/actions/workflows/test.yaml/badge.svg)
[![GoDoc](https://pkg.go.dev/github.com/dnaeon/go-vcr/v3)](https://pkg.go.dev/github.com/dnaeon/go-vcr/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/dnaeon/go-vcr)](https://goreportcard.com/report/github.com/dnaeon/go-vcr)
[![codecov](https://codecov.io/gh/dnaeon/go-vcr/branch/master/graph/badge.svg)](https://codecov.io/gh/dnaeon/go-vcr)

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
func customMatcher(r *http.Request, i Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return cassette.DefaultMatcher(r, i)
	}

	var reqBody []byte
	var err error
	reqBody, err = ioutil.ReadAll(r.Body)
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

## Protecting Sensitive Data

You often provide sensitive data, such as API credentials, when making
requests against a service.

By default, this data will be stored in the recorded data but you
probably don't want this.

Removing or replacing data before it is stored can be done by adding
one or more `Filter`s to your `Recorder`.

Here is an example that removes the `Authorization` header from all
requests:

```go
r, err := recorder.New("fixtures/filters")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

// Add a filter which removes Authorization headers from all requests:
r.AddFilter(func(i *cassette.Interaction) error {
    delete(i.Request.Headers, "Authorization")
    return nil
})
```

### Sensitive data in responses 

Filters added using `*Recorder.AddFilter` are applied within VCR's
custom `http.Transport`. This means that if you edit a response in
such a filter then subsequent test code will see the edited
response. This may not be desirable in all cases.

For instance, if a response body contains an OAuth access token that
is needed for subsequent requests, then redacting the access token in
`Filter` will result in authorization failures.

Another way to edit recorded interactions is to use
`PreSaveFilter`. Filters added with this method are applied just
before interactions are saved when `Recorder.Stop()` is called.

```go
r, err := recorder.New("fixtures/filters")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

// Your test code will continue to see the real access token and
// it is redacted before the recorded interactions are saved on disk
r.AddSaveFilter(func(i *cassette.Interaction) error {
    if strings.Contains(i.URL, "/oauth/token") {
        i.Response.Body = `{"access_token": "[REDACTED]"}`
    }

    return nil
})
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
