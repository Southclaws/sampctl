package analytics

import (
	"reflect"
	"testing"
)

func TestMessageIdDefault(t *testing.T) {
	if id := makeMessageId("", "42"); id != "42" {
		t.Error("invalid default message id:", id)
	}
}

func TestMessageIdNonDefault(t *testing.T) {
	if id := makeMessageId("A", "42"); id != "A" {
		t.Error("invalid non-default message id:", id)
	}
}

func TestMessageQueuePushMaxBatchSize(t *testing.T) {
	m0, _ := makeMessage(Track{
		UserId: "1",
		Event:  "A",
	}, maxMessageBytes)

	m1, _ := makeMessage(Track{
		UserId: "2",
		Event:  "A",
	}, maxMessageBytes)

	q := messageQueue{
		maxBatchSize:  2,
		maxBatchBytes: maxBatchBytes,
	}

	if msgs := q.push(m0); msgs != nil {
		t.Error("unexpected message batch returned after pushing only one message")
	}

	if msgs := q.push(m1); !reflect.DeepEqual(msgs, []message{m0, m1}) {
		t.Error("invalid message batch returned after pushing two messages:", msgs)
	}
}

func TestMessageQueuePushMaxBatchBytes(t *testing.T) {
	m0, _ := makeMessage(Track{
		UserId: "1",
		Event:  "A",
	}, maxMessageBytes)

	m1, _ := makeMessage(Track{
		UserId: "2",
		Event:  "A",
	}, maxMessageBytes)

	q := messageQueue{
		maxBatchSize:  100,
		maxBatchBytes: len(m0.json) + 1,
	}

	if msgs := q.push(m0); msgs != nil {
		t.Error("unexpected message batch returned after pushing only one message")
	}

	if msgs := q.push(m1); !reflect.DeepEqual(msgs, []message{m0}) {
		t.Error("invalid message batch returned after pushing two messages:", msgs)
	}

	if !reflect.DeepEqual(q.pending, []message{m1}) {
		t.Error("invalid state of the message queue after pushing two messages:", q.pending)
	}
}

func TestMakeMessage(t *testing.T) {
	track := Track{UserId: "1"}

	if msg, err := makeMessage(track, maxMessageBytes); err != nil {
		t.Error("failed to make message from track message:", err)

	} else if !reflect.DeepEqual(msg, message{
		msg:  track,
		json: []byte(`{"userId":"1","event":"","timestamp":"0001-01-01T00:00:00Z"}`),
	}) {
		t.Error("invalid message generated from track message:", msg.msg, string(msg.json))
	}
}

func TestMakeMessageTooBig(t *testing.T) {
	if _, err := makeMessage(Track{UserId: "1"}, 1); err != ErrMessageTooBig {
		t.Error("invalid error returned when creating a message bigger than the limit:", err)
	}
}
