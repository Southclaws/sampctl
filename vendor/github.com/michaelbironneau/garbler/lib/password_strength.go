/*The MIT License (MIT)

Copyright (c) 2015 Michael Bironneau

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.*/

package lib

import "unicode"

//PasswordStrengthRequirements describes the minimal requirements that the generated
//password must meet.
type PasswordStrengthRequirements struct {
	MinimumTotalLength int //Minimum number characters in password
	MaximumTotalLength int //Maximum number of characters (ignored if 0)
	Uppercase          int //Minimum number of uppercase letters
	Digits             int //Mininum number of digits
	Punctuation        int //Minimum number of special characters
}

//Validate a password against the given requirements
//Returns a boolean indicating whether the password meets the requirements.
//The second argument is a string explaining why it doesn't meet the requirements,
//if it doesn't. It is empty if the requirements are met.
func (p *PasswordStrengthRequirements) Validate(password string) (bool, string) {
	reqs := MakeRequirements(password)
	if p.MaximumTotalLength > 0 && reqs.MaximumTotalLength > p.MaximumTotalLength {
		return false, "password is too long"
	}
	if reqs.MinimumTotalLength < p.MinimumTotalLength {
		return false, "password is too short"
	}
	if reqs.Digits < p.Digits {
		return false, "password has too few digits"
	}
	if reqs.Punctuation < p.Punctuation {
		return false, "password has too few punctuation characters"
	}
	if reqs.Uppercase < p.Uppercase {
		return false, "password has too few uppercase characters"
	}
	return true, ""
}

//Generate password requirements from an existing password.
func MakeRequirements(password string) PasswordStrengthRequirements {
	pwd := []byte(password)
	reqs := PasswordStrengthRequirements{}
	reqs.MaximumTotalLength = len(password)
	reqs.MinimumTotalLength = len(password)
	for i := range pwd {
		switch {
		case unicode.IsDigit(rune(pwd[i])):
			reqs.Digits++
		case unicode.IsUpper(rune(pwd[i])):
			reqs.Uppercase++
		case unicode.IsPunct(rune(pwd[i])):
			reqs.Punctuation++
		}
	}
	return reqs
}

//Make sure password strength requirements make sense
func (p *PasswordStrengthRequirements) sanityCheck() (bool, string) {
	if p.MaximumTotalLength == 0 {
		return true, ""
	}
	if p.MaximumTotalLength < p.MinimumTotalLength {
		return false, "maximum total length is less than minimum total length"
	}
	if p.MaximumTotalLength < p.Digits {
		return false, "maximum required digits is more than maximum total length"
	}
	if p.MaximumTotalLength < p.Punctuation {
		return false, "maximum required punctuation is more than maximum total length"
	}
	if p.MaximumTotalLength < p.Uppercase {
		return false, "maximum required uppercase characters is more than maximum total length"
	}
	if p.MaximumTotalLength < p.Digits+p.Uppercase+p.Punctuation {
		return false, "maximum required digits + uppercase + punctuation is more than maximum total length"
	}
	return true, ""
}
