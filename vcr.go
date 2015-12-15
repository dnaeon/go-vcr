package vcr

import (
	"log"

	"github.com/dnaeon/go-vcr/recorder"
)

// Helper wrapper around recorder.Recorder type
// The returned Recorder must be stopped by the
// caller when no longer needed
func Play(cassette string) *recorder.Recorder {
	r := recorder.NewRecorder(cassette)

	err := r.Start()
	if err != nil {
		log.Fatal(err)
	}

	return r
}
