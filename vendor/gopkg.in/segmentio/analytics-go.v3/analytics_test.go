package analytics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper type used to implement the io.Reader interface on function values.
type readFunc func([]byte) (int, error)

func (f readFunc) Read(b []byte) (int, error) { return f(b) }

// Helper type used to implement the http.RoundTripper interface on function
// values.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (f roundTripperFunc) CancelRequest(r *http.Request) {}

// Instances of this type are used to mock the client callbacks in unit tests.
type testCallback struct {
	success func(Message)
	failure func(Message, error)
}

func (c testCallback) Success(m Message) {
	if c.success != nil {
		c.success(m)
	}
}

func (c testCallback) Failure(m Message, e error) {
	if c.failure != nil {
		c.failure(m, e)
	}
}

// Instances of this type are used to mock the client logger in unit tests.
type testLogger struct {
	logf   func(string, ...interface{})
	errorf func(string, ...interface{})
}

func (l testLogger) Logf(format string, args ...interface{}) {
	if l.logf != nil {
		l.logf(format, args...)
	}
}

func (l testLogger) Errorf(format string, args ...interface{}) {
	if l.errorf != nil {
		l.errorf(format, args...)
	}
}

// Instances of this type are used to force message validation errors in unit
// tests.
type testErrorMessage struct{}

func (m testErrorMessage) validate() error { return testError }

var (
	// A control error returned by mock functions to emulate a failure.
	testError = errors.New("test error")

	// HTTP transport that always succeeds.
	testTransportOK = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Proto:      r.Proto,
			ProtoMajor: r.ProtoMajor,
			ProtoMinor: r.ProtoMinor,
			Body:       ioutil.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	// HTTP transport that sleeps for a little while and eventually succeeds.
	testTransportDelayed = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		time.Sleep(10 * time.Millisecond)
		return testTransportOK.RoundTrip(r)
	})

	// HTTP transport that always returns a 400.
	testTransportBadRequest = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusBadRequest),
			StatusCode: http.StatusBadRequest,
			Proto:      r.Proto,
			ProtoMajor: r.ProtoMajor,
			ProtoMinor: r.ProtoMinor,
			Body:       ioutil.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	// HTTP transport that always returns a 400 with an erroring body reader.
	testTransportBodyError = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusBadRequest),
			StatusCode: http.StatusBadRequest,
			Proto:      r.Proto,
			ProtoMajor: r.ProtoMajor,
			ProtoMinor: r.ProtoMinor,
			Body:       ioutil.NopCloser(readFunc(func(b []byte) (int, error) { return 0, testError })),
			Request:    r,
		}, nil
	})

	// HTTP transport that always return an error.
	testTransportError = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, testError
	})
)

func fixture(name string) string {
	f, err := os.Open(filepath.Join("fixtures", name))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func mockId() string { return "I'm unique" }

func mockTime() time.Time {
	// time.Unix(0, 0) fails on Circle
	return time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
}

func mockServer() (chan []byte, *httptest.Server) {
	done := make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := bytes.NewBuffer(nil)
		io.Copy(buf, r.Body)

		var v interface{}
		err := json.Unmarshal(buf.Bytes(), &v)
		if err != nil {
			panic(err)
		}

		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			panic(err)
		}

		done <- b
	}))

	return done, server
}

func ExampleTrack() {
	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
	})

	fmt.Printf("%s\n", <-body)
	// Output:
	// {
	//   "batch": [
	//     {
	//       "event": "Download",
	//       "messageId": "I'm unique",
	//       "properties": {
	//         "application": "Segment Desktop",
	//         "platform": "osx",
	//         "version": "1.1.0"
	//       },
	//       "timestamp": "2009-11-10T23:00:00Z",
	//       "type": "track",
	//       "userId": "123456"
	//     }
	//   ],
	//   "context": {
	//     "library": {
	//       "name": "analytics-go",
	//       "version": "3.0.0"
	//     }
	//   },
	//   "messageId": "I'm unique",
	//   "sentAt": "2009-11-10T23:00:00Z"
	// }
}

func TestEnqueue(t *testing.T) {
	tests := map[string]struct {
		ref string
		msg Message
	}{
		"alias": {
			fixture("test-enqueue-alias.json"),
			Alias{PreviousId: "A", UserId: "B"},
		},

		"group": {
			fixture("test-enqueue-group.json"),
			Group{GroupId: "A", UserId: "B"},
		},

		"identify": {
			fixture("test-enqueue-identify.json"),
			Identify{UserId: "B"},
		},

		"page": {
			fixture("test-enqueue-page.json"),
			Page{Name: "A", UserId: "B"},
		},

		"screen": {
			fixture("test-enqueue-screen.json"),
			Screen{Name: "A", UserId: "B"},
		},

		"track": {
			fixture("test-enqueue-track.json"),
			Track{
				Event:  "Download",
				UserId: "123456",
				Properties: Properties{
					"application": "Segment Desktop",
					"version":     "1.1.0",
					"platform":    "osx",
				},
			},
		},
	}

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	for name, test := range tests {
		if err := client.Enqueue(test.msg); err != nil {
			t.Error(err)
			return
		}

		if res := string(<-body); res != test.ref {
			t.Errorf("%s: invalid response:\n- expected %s\n- received: %s", name, test.ref, res)
		}
	}
}

func TestTrackWithInterval(t *testing.T) {
	const interval = 100 * time.Millisecond
	var ref = fixture("test-interval-track.json")

	body, server := mockServer()
	defer server.Close()

	t0 := time.Now()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint: server.URL,
		Interval: interval,
		Verbose:  true,
		Logger:   t,
		now:      mockTime,
		uid:      mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
	})

	// Will flush in 100 milliseconds
	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}

	if t1 := time.Now(); t1.Sub(t0) < interval {
		t.Error("the flushing interval is too short:", interval)
	}
}

func TestTrackWithTimestamp(t *testing.T) {
	var ref = fixture("test-timestamp-track.json")

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
		Timestamp: time.Date(2015, time.July, 10, 23, 0, 0, 0, time.UTC),
	})

	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}
}

func TestTrackWithMessageId(t *testing.T) {
	var ref = fixture("test-messageid-track.json")

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
		MessageId: "abc",
	})

	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}
}

func TestTrackWithContext(t *testing.T) {
	var ref = fixture("test-context-track.json")

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
		Context: &Context{
			Extra: map[string]interface{}{
				"whatever": "here",
			},
		},
	})

	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}
}

func TestTrackMany(t *testing.T) {
	var ref = fixture("test-many-track.json")

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 3,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	for i := 0; i < 5; i++ {
		client.Enqueue(Track{
			Event:  "Download",
			UserId: "123456",
			Properties: Properties{
				"application": "Segment Desktop",
				"version":     i,
			},
		})
	}

	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}
}

func TestTrackWithIntegrations(t *testing.T) {
	var ref = fixture("test-integrations-track.json")

	body, server := mockServer()
	defer server.Close()

	client, _ := NewWithConfig("h97jamjwbh", Config{
		Endpoint:  server.URL,
		Verbose:   true,
		Logger:    t,
		BatchSize: 1,
		now:       mockTime,
		uid:       mockId,
	})
	defer client.Close()

	client.Enqueue(Track{
		Event:  "Download",
		UserId: "123456",
		Properties: Properties{
			"application": "Segment Desktop",
			"version":     "1.1.0",
			"platform":    "osx",
		},
		Integrations: Integrations{
			"All":      true,
			"Intercom": false,
			"Mixpanel": true,
		},
	})

	if res := string(<-body); ref != res {
		t.Errorf("invalid response:\n- expected %s\n- received: %s", ref, res)
	}
}

func TestClientCloseTwice(t *testing.T) {
	client := New("0123456789")

	if err := client.Close(); err != nil {
		t.Error("closing a client should not a return an error")
	}

	if err := client.Close(); err != ErrClosed {
		t.Error("closing a client a second time should return ErrClosed:", err)
	}

	if err := client.Enqueue(Track{UserId: "1", Event: "A"}); err != ErrClosed {
		t.Error("using a client after it was closed should return ErrClosed:", err)
	}
}

func TestClientConfigError(t *testing.T) {
	client, err := NewWithConfig("0123456789", Config{
		Interval: -1 * time.Second,
	})

	if err == nil {
		t.Error("no error returned when creating a client with an invalid config")
	}

	if _, ok := err.(ConfigError); !ok {
		t.Errorf("invalid error type returned when creating a client with an invalid config: %T", err)
	}

	if client != nil {
		t.Error("invalid non-nil client object returned when creating a client with and invalid config:", client)
		client.Close()
	}
}

func TestClientEnqueueError(t *testing.T) {
	client := New("0123456789")
	defer client.Close()

	if err := client.Enqueue(testErrorMessage{}); err != testError {
		t.Error("invlaid error returned when queueing an invalid message:", err)
	}
}

func TestClientCallback(t *testing.T) {
	reschan := make(chan bool, 1)
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			func(m Message) { reschan <- true },
			func(m Message, e error) { errchan <- e },
		},
		Transport: testTransportOK,
	})

	client.Enqueue(Track{
		UserId: "A",
		Event:  "B",
	})
	client.Close()

	select {
	case <-reschan:
	case err := <-errchan:
		t.Error("failure callback triggered:", err)
	}
}

func TestClientMarshalMessageError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		Transport: testTransportOK,
	})

	// Functions cannot be serializable, this should break the JSON marshaling
	// and trigger the failure callback.
	client.Enqueue(Track{
		UserId:     "A",
		Event:      "B",
		Properties: Properties{"invalid": func() {}},
	})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for unserializable message")

	} else if _, ok := err.(*json.UnsupportedTypeError); !ok {
		t.Errorf("invalid error type returned by unserializable message: %T", err)
	}
}

func TestClientMarshalContextError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		DefaultContext: &Context{
			// The context set on the batch message is invalid this should also
			// cause the batched message to fail to be serialized and call the
			// failure callback.
			Extra: map[string]interface{}{"invalid": func() {}},
		},
		Transport: testTransportOK,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for unserializable context")

	} else if _, ok := err.(*json.MarshalerError); !ok {
		t.Errorf("invalid error type returned by unserializable context: %T", err)
	}
}

func TestClientNewRequestError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Endpoint: "://localhost:80", // Malformed endpoint URL.
		Logger:   testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		Transport: testTransportOK,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for an invalid request")
	}
}

func TestClientRoundTripperError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		Transport: testTransportError,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for an invalid request")

	} else if e, ok := err.(*url.Error); !ok {
		t.Errorf("invalid error returned by round tripper: %T: %s", err, err)

	} else if e.Err != testError {
		t.Errorf("invalid error returned by round tripper: %T: %s", e.Err, e.Err)
	}
}

func TestClientRetryError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, testError
		}),
		BatchSize:  1,
		RetryAfter: func(i int) time.Duration { return time.Millisecond },
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})

	// Each retry should happen ~1 millisecond, this should give enough time to
	// the test to trigger the failure callback.
	time.Sleep(50 * time.Millisecond)

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for a retry falure")

	} else if e, ok := err.(*url.Error); !ok {
		t.Errorf("invalid error returned by round tripper: %T: %s", err, err)

	} else if e.Err != testError {
		t.Errorf("invalid error returned by round tripper: %T: %s", e.Err, e.Err)
	}

	client.Close()
}

func TestClientResponse400(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		// This HTTP transport always return 400's.
		Transport: testTransportBadRequest,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for a 400 response")
	}
}

func TestClientResponseBodyError(t *testing.T) {
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			nil,
			func(m Message, e error) { errchan <- e },
		},
		// This HTTP transport always return 400's with an erroring body.
		Transport: testTransportBodyError,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered for a 400 response")

	} else if err != testError {
		t.Errorf("invalid error returned by erroring response body: %T: %s", err, err)
	}
}

func TestClientMaxConcurrentRequests(t *testing.T) {
	reschan := make(chan bool, 1)
	errchan := make(chan error, 1)

	client, _ := NewWithConfig("0123456789", Config{
		Logger: testLogger{t.Logf, t.Logf},
		Callback: testCallback{
			func(m Message) { reschan <- true },
			func(m Message, e error) { errchan <- e },
		},
		Transport: testTransportDelayed,
		// Only one concurreny request can be submitted, because the transport
		// introduces a short delay one of the uploads should fail.
		BatchSize:             1,
		maxConcurrentRequests: 1,
	})

	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Enqueue(Track{UserId: "A", Event: "B"})
	client.Close()

	if _, ok := <-reschan; !ok {
		t.Error("one of the requests should have succeeded but the result channel was empty")
	}

	if err := <-errchan; err == nil {
		t.Error("failure callback not triggered after reaching the request limit")

	} else if err != ErrTooManyRequests {
		t.Errorf("invalid error returned by erroring response body: %T: %s", err, err)
	}
}
