# Garbler

*Author*: Michael Bironneau
*License*: MIT

Generator of memorable passwords, written in Go. Available both as a command-line tool and as a Go library.

Please see [here](http://godoc.org/github.com/michaelbironneau/garbler/lib) for Godocs or read on for examples.

**What makes the passwords memorable?**

The password generation method is inspired by the environ passwords, which are designed to be pronouncable, and therefore easier to memorize. For extra security, the method originally used to generate environ passwords has been generalized to meet any password strength requirement that can be expressed in minimum/maximum length, minimum number of digits, minimum number of punctuation characters, and/or minimum number of uppercase letters.

A few example passwords:

* Denwiqil628-
* Riwwabolmi556:
* .!Bonhomiqvi984-:!
* ZizqoRuK293?

Garbler has no external dependencies and is fast enough for bulk use: it can generate 10-20k passwords per second depending on the preset used (Intel core i7 Pro).

```
BenchmarkParanoid	   10000	    193711 ns/op
BenchmarkMedium	       20000	     88755 ns/op
```

## Installation

Run `go get github.com/michaelbironneau/garbler`. If you want to use it as a command line tool, you will also need to run `go install` to get the binaries. I can provide links to pre-built binaries if there is enough interest - please open an issue if you would like this to happen.


## Code examples

The API is simple, clean, and comes with presets and sensible defaults:
```go
import (
	garbler "github.com/michaelbironneau/garbler/lib"
	"fmt"
)

func main() {
	//use defaults
	p, _ := garbler.NewPassword(nil)
	fmt.Println(p)

	//use Strong preset (Insecure/Easy/Medium/Strong/Paranoid are available)
	p, _ = garbler.NewPassword(&garbler.Strong)
	fmt.Println(p)

	//guess requirements from existing password
	reqs := garbler.MakeRequirements("asdfGG11!")
	p, _ = garbler.NewPassword(&reqs)
	fmt.Println(p)

	//specify requirements explicitly:
	//if specifying requirements you should not ignore error return,
	//in case the requirements are impossible to satisfy (eg. minimum length is
    //greater than maximum length)
	reqs = garbler.PasswordStrengthRequirements{MinimumTotalLength: 20, Digits:10}
	p, e := garbler.NewPassword(&reqs)
	if e != nil {
		fmt.Println(e)
		return
	}
	fmt.Println(p)
}
```

**Generating multiple passwords**

You can use `NewPasswords` instead of `NewPassword` to get a string slice of passwords:
```go
passwords, _ := garbler.NewPasswords(&garbler.Strong, 10) //generates 10 passwords with Strong preset
```

**Using Garbler to check password strength**

You can also use Garbler to check a given password against strength requirements. For example,

```go
reqs := PasswordStrengthRequirements{MinimumTotalLength: 8, Punctuation: 1, Uppercase: 1, Digits: 1}
if ok, message := reqs.Validate(password); !ok {
	fmt.Println("your password failed validation because" + message)
}
```

## Command-line usage

*This assumes that `$GOPATH/bin` has been added to your `$PATH` environment variable. If that is not the case, you should either do that or first change your current working directory to `$GOPATH/bin` before attempting the following.*

The `garbler` command, without any arguments, will spit out 10 passwords that:

* are 12 characters long
* have 3 digits
* have 1 punctuation character
* have 1 uppercase character

You can use the following flags to modify the behavior:

* `n` : number of passwords to generate (eg. `garbler -n=20`)
* `min`: minimum length of generated password (eg. `garbler -min=10`)
* `max`: maximum length of generated password (eg. `garbler -max=10`)
* `digits`: minimum number of digits in password (eg. `garbler -min=15 -max=20 -digits=5`)
* `punctuation`: minimum number of punctuation characters (eg. `garbler -punctuation=5`)
* `uppercase`: minimum number of uppercase characters (eg. `garbler -uppercase=3`)

## Golang API

To use Garbler within your Go application, install it as described above, then import `github.com/michaelbironneau/garbler/lib`, as in the example.

See the godocs [here](http://godoc.org/github.com/michaelbironneau/garbler/lib).
