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

//Garbler is a package to generate memorable passwords
package lib

import (
	crypto "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"
	"unicode"
)

//Garbler is a generator that creates a generalized environ sequence to
//the specified requirement. It then garbles the password
//(i.e. replacing a letter by a similar-looking number)
type Garbler struct{}

var Vowels, GarblableVowels, VowelGarblers, Consonants, GarblableConsonants, ConsonantGarblers, Punctuation []rune

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	Vowels = []rune("aeiou")
	GarblableVowels = []rune("eio")
	VowelGarblers = []rune("310")
	Consonants = []rune("bcdfghjklmnpqrstvwxyz")
	GarblableConsonants = []rune("bdgls")
	ConsonantGarblers = []rune("86915")
	Punctuation = []rune("!-.,?;:/^+=_*|\"<>[]{}`'()@&$#%")
}

//Generate a password given requirements
func (g Garbler) password(req PasswordStrengthRequirements) (string, error) {
	//Step 1: Figure out settings
	letters := 0
	mustGarble := 0
	switch {
	case req.MaximumTotalLength > 0 && req.MaximumTotalLength > 6:
		letters = req.MaximumTotalLength - req.Digits - req.Punctuation
	case req.MaximumTotalLength > 0 && req.MaximumTotalLength <= 6:
		letters = req.MaximumTotalLength - req.Punctuation
		mustGarble = req.Digits
	case req.MinimumTotalLength > req.Digits+req.Punctuation+6:
		letters = req.MinimumTotalLength - req.Digits - req.Punctuation
	default:
		letters = req.MinimumTotalLength
	}
	if req.Uppercase > letters {
		letters = req.Uppercase
	}
	password := g.garbledSequence(letters, mustGarble)
	password = g.uppercase(password, req.Uppercase)
	password = g.addNums(password, req.Digits-mustGarble)
	password = g.punctuate(password, req.Punctuation)
	return password, nil
}

//Generate one password that meets the given requirements
func NewPassword(reqs *PasswordStrengthRequirements) (string, error) {
	if reqs == nil {
		reqs = &Medium
	}
	if ok, problems := reqs.sanityCheck(); !ok {
		return "", errors.New("requirements failed validation: " + problems)
	}
	e := Garbler{}
	return e.password(*reqs)
}

//Generate n passwords that meet the given requirements
func NewPasswords(reqs *PasswordStrengthRequirements, n int) ([]string, error) {
	var err error
	if reqs == nil {
		reqs = &Medium
	}
	if ok, problems := reqs.sanityCheck(); !ok {
		return nil, errors.New("requirements failed validation: " + problems)
	}
	e := Garbler{}
	passes := make([]string, n, n)
	for i := 0; i < n; i++ {
		passes[i], err = e.password(*reqs)
		if err != nil {
			return nil, err
		}
	}
	return passes, nil
}

//append digits to string
func (g Garbler) addNums(p string, numDigits int) string {
	if numDigits <= 0 {
		return p
	}
	ret := p
	remaining := numDigits
	for remaining > 10 {
		ret += fmt.Sprintf("%d", pow(10, 9)+randInt(pow(10, 10)-pow(10, 9)))
		remaining -= 10
	}
	ret += fmt.Sprintf("%d", pow(10, remaining-1)+randInt(pow(10, remaining)-pow(10, remaining-1)))

	return ret
}

//add punctuation characters to start and end of string
func (g Garbler) punctuate(p string, numPunc int) string {
	if numPunc <= 0 {
		return p
	}
	ret := p
	for i := 0; i < numPunc; i++ {
		if i%2 == 0 {
			ret += string(Punctuation[randInt(len(Punctuation))])
		} else {
			ret = string(Punctuation[randInt(len(Punctuation))]) + ret
		}
	}
	return ret
}

//the environ sequence is:
//consonant, vowel, consonant, consonant, vowel, [some other stuff]
//we generalize it by removing [some other stuff] and allowing the sequence
//to repeat arbitrarily often. we also allow garbling and adding some extra
//digits.
func (g Garbler) garbledSequence(length int, numGarbled int) string {
	if numGarbled > length {
		panic("should not require more garbled chars than string length")
	}
	var ret string
	numCanGarble := 0
	sequence := []string{"c", "v", "c", "c", "v"}
	sequencePosition := 0
	for i := 0; i < length; i++ {
		if i%2 == 0 && numCanGarble < numGarbled {
			//make things garblable if required:
			//make every other character garblable until we reach numGarblable
			if sequence[sequencePosition] == "c" {
				ret += string(ConsonantGarblers[randInt(len(ConsonantGarblers))])
			} else {
				ret += string(VowelGarblers[randInt(len(VowelGarblers))])
			}
			numCanGarble++
			sequencePosition = (sequencePosition + 1) % len(sequence)
			continue
		}
		//no need to garble this character, just generate a random vowel/consonant
		if sequence[sequencePosition] == "c" {
			ret += string(Consonants[randInt(len(Consonants))])
		} else {
			ret += string(Vowels[randInt(len(Vowels))])
		}
		sequencePosition = (sequencePosition + 1) % len(sequence)
	}
	if numCanGarble >= numGarbled {
		return ret
	}
	//we've made even-numbered chars garbled, now start with the odd-numbered ones
	for i := 0; i < length; i++ {
		if i%2 == 1 && numCanGarble < numGarbled {
			//make things garblable if required:
			//make every other character garblable until we reach numGarblable
			if sequence[sequencePosition] == "c" {
				ret += string(ConsonantGarblers[randInt(len(ConsonantGarblers))])
			} else {
				ret += string(VowelGarblers[randInt(len(VowelGarblers))])
			}
			numCanGarble++
			sequencePosition = (sequencePosition + 1) % len(sequence)
		} else if numCanGarble >= numGarbled {
			return ret
		}
	}
	//if we reach this point, something went horribly wrong
	panic("ouch")
}

func (g Garbler) uppercase(p string, numUppercase int) string {
	if numUppercase <= 0 {
		return p
	}
	b := []byte(p)
	numsDone := 0
	for i := 0; i < len(b); i++ {
		//play nice with environ sequence,
		//just uppercase 1st and 2nd consonants,
		//which should make the whole thing more readable
		if i%5 == 0 || i%5 == 2 {
			b[i] = byte(unicode.ToUpper(rune(b[i])))
			numsDone++
			if numsDone >= numUppercase {
				return string(b)
			}
		}
	}
	//playing nice didn't work out so do the other letters too
	//in no particular order
	for i := 0; i < len(b); i++ {
		if !(i%5 == 0 || i%5 == 2) {
			b[i] = byte(unicode.ToUpper(rune(b[i])))
			numsDone++
			if numsDone >= numUppercase {
				return string(b)
			}
		}
	}
	//still here? then numUppercase was too large, panic
	panic("ouch")
}

//because Go doesn't have integer exponentiation function
func pow(a, b int) int {
	p := 1
	for b > 0 {
		if b&1 != 0 {
			p *= a
		}
		b >>= 1
		a *= a
	}
	return p
}

//best-effort attempt to get an int from crypto/rand. if
//an error is returned, it will fall back to math/rand.
func randInt(max int) int {
	i, err := crypto.Int(crypto.Reader, big.NewInt(int64(max)))
	if err == nil {
		return int(i.Int64())
	}
	return rand.Intn(max)
}
