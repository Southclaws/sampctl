package analytics

import "testing"

func TestScreenMissingUserId(t *testing.T) {
	screen := Screen{}

	if err := screen.validate(); err == nil {
		t.Error("validating an invalid screen object succeeded:", screen)

	} else if e, ok := err.(FieldError); !ok {
		t.Error("invalid error type returned when validating screen:", err)

	} else if e != (FieldError{
		Type:  "analytics.Screen",
		Name:  "UserId",
		Value: "",
	}) {
		t.Error("invalid error value returned when validating screen:", err)
	}
}

func TestScreenValidWithUserId(t *testing.T) {
	screen := Screen{
		UserId: "2",
	}

	if err := screen.validate(); err != nil {
		t.Error("validating a valid screen object failed:", screen, err)
	}
}

func TestScreenValidWithAnonymousId(t *testing.T) {
	screen := Screen{
		AnonymousId: "2",
	}

	if err := screen.validate(); err != nil {
		t.Error("validating a valid screen object failed:", screen, err)
	}
}
