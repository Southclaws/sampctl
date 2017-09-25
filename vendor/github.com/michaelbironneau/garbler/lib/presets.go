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

//Presets for password strength requirements. Medium or Strong are recommended.
var Insecure, Easy, Medium, Strong, Paranoid PasswordStrengthRequirements

func init() {
	Insecure = PasswordStrengthRequirements{
		MinimumTotalLength: 6,
	}
	//Meets Cyber Essentials requirements
	Easy = PasswordStrengthRequirements{
		MinimumTotalLength: 8,
		Digits:             3,
	}
	//Meets Wikipedia entry on "password strengh requirements"
	//requirements
	Medium = PasswordStrengthRequirements{
		MinimumTotalLength: 12,
		Uppercase:          4,
		Digits:             4,
		Punctuation:        2,
	}
	//Loosely based on Bruce Schneier's recommendations - when
	//used with the Garbler generator it will produce a password that
	//cannot be cracked by the PRTK program described here:
	//https://www.schneier.com/blog/archives/2007/01/choosing_secure.html
	Strong = PasswordStrengthRequirements{
		MinimumTotalLength: 16,
		Uppercase:          5,
		Digits:             6,
		Punctuation:        4,
	}
	//For super-top-secret spying and those among us who think they may have
	//at some point been abducted by aliens
	Paranoid = PasswordStrengthRequirements{
		MinimumTotalLength: 32,
		Uppercase:          12,
		Digits:             12,
		Punctuation:        8,
	}
}
