## go-vcr

[![Build Status](https://travis-ci.org/dnaeon/go-vcr.svg)](https://travis-ci.org/dnaeon/go-vcr)
[![GoDoc](https://godoc.org/github.com/dnaeon/go-vcr?status.svg)](https://godoc.org/github.com/dnaeon/go-vcr)
[![Go Report Card](https://goreportcard.com/badge/github.com/dnaeon/go-vcr)](https://goreportcard.com/report/github.com/dnaeon/go-vcr)
[![codecov](https://codecov.io/gh/dnaeon/go-vcr/branch/master/graph/badge.svg)](https://codecov.io/gh/dnaeon/go-vcr)

`go-vcr` simplifies testing by recording your HTTP interactions and
replaying them in future runs in order to provide fast, deterministic
and accurate testing of your code.

`go-vcr` was inspired by the [VCR library for Ruby](https://github.com/vcr/vcr).

## Installation

Install `go-vcr` by executing the command below:

```bash
$ go get github.com/dnaeon/go-vcr/recorder
```

## Usage

Here is a simple example of recording and replaying
[etcd](https://github.com/coreos/etcd) HTTP interactions.

You can find other examples in the `example` directory of this
repository as well.

```go
package main

import (
	"log"
	"time"

	"github.com/dnaeon/go-vcr/recorder"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

func main() {
	// Start our recorder
	r, err := recorder.New("fixtures/etcd")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Stop() // Make sure recorder is stopped once done with it

	// Create an etcd configuration using our transport
	cfg := client.Config{
		Endpoints:               []string{"http://127.0.0.1:2379"},
		HeaderTimeoutPerRequest: time.Second,
		Transport:               r, // Inject as transport!
	}

	// Create an etcd client using the above configuration
	c, err := client.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create etcd client: %s", err)
	}

	// Get an example key from etcd
	etcdKey := "/foo"
	kapi := client.NewKeysAPI(c)
	resp, err := kapi.Get(context.Background(), etcdKey, nil)

	if err != nil {
		log.Fatalf("Failed to get etcd key %s: %s", etcdKey, err)
	}

	log.Printf("Successfully retrieved etcd key %s: %s", etcdKey, resp.Node.Value)
}
```

## Custom Request Matching

During replay mode, You can customize the way incoming requests are
matched against the recorded request/response pairs by defining a
Matcher function. For example, the following matcher will match on
method, URL and body:

```go
r, err := recorder.New("fixtures/matchers")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
	if r.Body == nil {
		return cassette.DefaultMatcher(r, i)
	}
	var b bytes.Buffer
	if _, err := b.ReadFrom(r.Body); err != nil {
		return false
	}
	r.Body = ioutil.NopCloser(&b)
	return cassette.DefaultMatcher(r, i) && (b.String() == "" || b.String() == i.Body)
})
```

## Protecting Sensitive Data

You often provide sensitive data, such as API credentials, when making
requests against a service.
By default, this data will be stored in the recorded data but you probably
don't want this.
Removing or replacing data before it is stored can be done by adding one or
more `Filter`s to your `Recorder`.
Here is an example that removes the `Authorization` header from all requests:

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

Filters added using `*Recorder.AddFilter` are applied within VCR's custom `http.Transport`. This means that if you edit a response in such a filter then subsequent test code will see the edited response. This may not be desirable in all cases. For instance, if a response body contains an OAuth access token that is needed for subsequent requests, then redact the access token in `SaveFilter` will result in authorization failures.

Another way to edit recorded interactions is to use `*Recorder.AddSaveFilter`. Filters added with this method are applied just before interactions are saved when `*Recorder.Stop` is called.

```go
r, err := recorder.New("fixtures/filters")
if err != nil {
	log.Fatal(err)
}
defer r.Stop() // Make sure recorder is stopped once done with it

// Your test code will continue to see the real access token and
// it is redacted before the recorded interactions are saved     
r.AddSaveFilter(func(i *cassette.Interaction) error {
    if strings.Contains(i.URL, "/oauth/token") {
        i.Response.Body = `{"access_token": "[REDACTED]"}`
    }

    return nil
})
```    

## Passing Through Requests

Sometimes you want to allow specific requests to pass through to the remote
server without recording anything.

Globally, you can use `ModeDisabled` for this, but if you want to disable the
recorder for individual requests, you can add `Passthrough` functions to the
recorder. The function takes a pointer to the original request, and returns a
boolean, indicating if the request should pass through to the remote server.

Here's an example to pass through requests to a specific endpoint:

```go
// Pass through the request to the remote server if the path matches "/login".
r.AddPassthrough(func(req *http.Request) bool {
    return req.URL.Path == "/login"
})
```

## License

`go-vcr` is Open Source and licensed under the
[BSD License](http://opensource.org/licenses/BSD-2-Clause)
