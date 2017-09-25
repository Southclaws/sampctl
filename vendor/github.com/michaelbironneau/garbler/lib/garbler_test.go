package lib

import (
	"testing"
)

func Test1(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, MaximumTotalLength: 8}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func Test2(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Digits: 3}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func Test3(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Uppercase: 3}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func Test4(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Punctuation: 3}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestManyDigits(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Digits: 30}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestEasy(t *testing.T) {
	reqs := Easy
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestLength(t *testing.T) {
	reqs := Easy
	p, e := NewPasswords(&reqs, 10)
	if e != nil {
		t.Error(e)
	}
	if len(p) != 10 {
		t.Error("expected 10 passwords, got", len(p))
	}
}

func TestDifferent(t *testing.T) {
	reqs := Easy
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	q, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if p == q {
		t.Error("got the same password twice. run the tests again, if it gives the same failure it is probably a bug")
	}
}

func TestMedium(t *testing.T) {
	reqs := Medium
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestStrong(t *testing.T) {
	reqs := Strong
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestParanoid(t *testing.T) {
	reqs := Paranoid
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func TestLotsOfUppercase(t *testing.T) {
	reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Uppercase: 10}
	p, e := NewPassword(&reqs)
	if e != nil {
		t.Error(e)
	}
	if ok, msg := reqs.Validate(p); !ok {
		t.Error(msg)
	}
}

func BenchmarkParanoid(b *testing.B) {
	reqs := Paranoid //worst-case from presets
	for n := 0; n < b.N; n++ {
		_, e := NewPassword(&reqs)
		if e != nil {
			b.Error(e)
		}
	}
}

func BenchmarkMedium(b *testing.B) {
	reqs := Medium //worst-case from presets
	for n := 0; n < b.N; n++ {
		_, e := NewPassword(&reqs)
		if e != nil {
			b.Error(e)
		}
	}
}
