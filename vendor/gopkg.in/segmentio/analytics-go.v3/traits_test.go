package analytics

import (
	"reflect"
	"testing"
	"time"
)

func TestTraitsSimple(t *testing.T) {
	date := time.Now()
	text := "ABC"
	number := 42

	tests := map[string](struct {
		ref Traits
		run func(Traits)
	}){
		"address":     {Traits{"address": text}, func(t Traits) { t.SetAddress(text) }},
		"age":         {Traits{"age": number}, func(t Traits) { t.SetAge(number) }},
		"avatar":      {Traits{"avatar": text}, func(t Traits) { t.SetAvatar(text) }},
		"birthday":    {Traits{"birthday": date}, func(t Traits) { t.SetBirthday(date) }},
		"createdAt":   {Traits{"createdAt": date}, func(t Traits) { t.SetCreatedAt(date) }},
		"description": {Traits{"description": text}, func(t Traits) { t.SetDescription(text) }},
		"email":       {Traits{"email": text}, func(t Traits) { t.SetEmail(text) }},
		"firstName":   {Traits{"firstName": text}, func(t Traits) { t.SetFirstName(text) }},
		"lastName":    {Traits{"lastName": text}, func(t Traits) { t.SetLastName(text) }},
		"gender":      {Traits{"gender": text}, func(t Traits) { t.SetGender(text) }},
		"name":        {Traits{"name": text}, func(t Traits) { t.SetName(text) }},
		"phone":       {Traits{"phone": text}, func(t Traits) { t.SetPhone(text) }},
		"title":       {Traits{"title": text}, func(t Traits) { t.SetTitle(text) }},
		"username":    {Traits{"username": text}, func(t Traits) { t.SetUsername(text) }},
		"website":     {Traits{"website": text}, func(t Traits) { t.SetWebsite(text) }},
	}

	for name, test := range tests {
		traits := NewTraits()
		test.run(traits)

		if !reflect.DeepEqual(traits, test.ref) {
			t.Errorf("%s: invalid traits produced: %#v\n", name, traits)
		}
	}
}

func TestTraitsMulti(t *testing.T) {
	t0 := Traits{"firstName": "Luke", "lastName": "Skywalker"}
	t1 := NewTraits().SetFirstName("Luke").SetLastName("Skywalker")

	if !reflect.DeepEqual(t0, t1) {
		t.Errorf("invalid traits produced by chained setters:\n- expected %#v\n- found: %#v", t0, t1)
	}
}
