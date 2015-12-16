package vcr_test

import (
	"testing"
	"time"

	"github.com/dnaeon/go-vcr/recorder"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

func TestEtcd(t *testing.T) {
	// Start our recorder
	r, err := recorder.New("fixtures/etcd")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop() // Make sure recorder is stopped once done with it

	// Create an etcd configuration using our transport
	cfg := client.Config{
		Endpoints:               []string{"http://127.0.0.1:2379"},
		HeaderTimeoutPerRequest: time.Second,
		Transport:               r.Transport, // Inject our transport!
	}

	// Create an etcd client using the above configuration
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create etcd client: %s", err)
	}

	// Get an example key from etcd
	kapi := client.NewKeysAPI(c)
	resp, err := kapi.Get(context.Background(), "/foo", nil)
	if err != nil {
		t.Fatalf("Failed to get etcd key: %s", err)
	}

	wantValue := "bar"
	gotValue := resp.Node.Value

	if wantValue != gotValue {
		t.Errorf("want %q value, got %q value", wantValue, gotValue)
	}
}
