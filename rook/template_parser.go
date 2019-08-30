package rook

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/AlecAivazis/survey.v1"
)

// GenerateFileFromTemplate will ask the user questions (if needed) about a template.
// Then, when the user has finished all of them, we will generate their entry point file.
func GenerateFileFromTemplate(templatepath string) (bytes.Buffer, error) {
	templateHandle, err := os.Open(templatepath)
	if err != nil {
		return bytes.Buffer{}, err
	}

	bytesOutput := bytes.Buffer{}
	fileScanner := bufio.NewScanner(templateHandle)

	for fileScanner.Scan() {
		line := fileScanner.Text()
		isStringOption := false
		if strings.Contains(line, "#define") {
			defineSplit := strings.Split(line, " ")

			var tmpSplit []string
			for _, str := range defineSplit {
				if str != "" {
					tmpSplit = append(tmpSplit, str)
				}
			}
			defineSplit = tmpSplit

			if strings.Contains(defineSplit[2], "\"") && strings.Contains(defineSplit[len(defineSplit)-1], "\"") {
				defineSplit[2] = strings.Join(defineSplit[2:], " ")
				// Remove quotation marks.
				defineSplit[2] = strings.Replace(defineSplit[2], "\"", "", -1)
				isStringOption = true
			}

			response, err := askUserInputQuestion(fmt.Sprintf("Set "+defineSplit[1]+"?"), defineSplit[2])
			if err != nil {
				return bytes.Buffer{}, err
			}

			// Add the quotation marks to response.
			if isStringOption == true {
				response = "\"" + response + "\""
			}

			bytesOutput.WriteString(defineSplit[0] + " " + defineSplit[1] + " " + response + "\n")
		} else {
			bytesOutput.WriteString(line + "\n")
		}
	}

	return bytesOutput, nil
}

func askUserInputQuestion(question string, defaultvalue string) (string, error) {
	type answer struct {
		Response string
	}

	var questions = []*survey.Question{
		{
			Name: "Response",
			Prompt: &survey.Input{
				Message: question,
				Default: defaultvalue,
			},
			Validate: survey.Required,
		},
	}

	answers := answer{}
	err := survey.Ask(questions, &answers)

	return answers.Response, err
}
