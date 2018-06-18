package analytics

import "testing"

func TestPageMissingUserId(t *testing.T) {
	page := Page{}

	if err := page.validate(); err == nil {
		t.Error("validating an invalid page object succeeded:", page)

	} else if e, ok := err.(FieldError); !ok {
		t.Error("invalid error type returned when validating page:", err)

	} else if e != (FieldError{
		Type:  "analytics.Page",
		Name:  "UserId",
		Value: "",
	}) {
		t.Error("invalid error value returned when validating page:", err)
	}
}

func TestPageValidWithUserId(t *testing.T) {
	page := Page{
		UserId: "2",
	}

	if err := page.validate(); err != nil {
		t.Error("validating a valid page object failed:", page, err)
	}
}

func TestPageValidWithAnonymousId(t *testing.T) {
	page := Page{
		AnonymousId: "2",
	}

	if err := page.validate(); err != nil {
		t.Error("validating a valid page object failed:", page, err)
	}
}
