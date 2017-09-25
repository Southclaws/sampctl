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

//Garbler command-line tool
package main

import (
	"flag"
	"fmt"
	garbler "github.com/michaelbironneau/garbler/lib"
)

//simple CLI interface. Sample usage:
//goner -n=8 -min=8 -max=10 -digits=3 -punctuation=3 -uppercase=4
//Flags:
//  -n: number of passwords to generate
//  -min: minimum length
//  -max: maximum length
//  -digits: digits (0-9) characters
//  -punctuation: punctuation characters
//  -uppercase: uppercase characters
func main() {
	n := flag.Int("n", 8, "number of passwords to generate")
	min := flag.Int("min", 12, "minimum password length")
	max := flag.Int("max", 0, "maximum password length")
	digits := flag.Int("digits", 3, "number of digits")
	punctuation := flag.Int("punctuation", 1, "number of punctuation symbols")
	uppercase := flag.Int("uppercase", 1, "number of uppercase characters")
	flag.Parse()
	reqs := garbler.PasswordStrengthRequirements{
		MinimumTotalLength: *min,
		MaximumTotalLength: *max,
		Uppercase:          *uppercase,
		Digits:             *digits,
		Punctuation:        *punctuation,
	}
	for i := 0; i < *n; i++ {
		pass, err := garbler.NewPassword(&reqs)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(pass)
	}
}
