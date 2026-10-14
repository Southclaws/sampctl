package rook

import "github.com/AlecAivazis/survey/v2"

func requiredSelection(answer interface{}) error {
	// An unmatched selection can have index -1, so validate its value.
	if selection, ok := answer.(survey.OptionAnswer); ok {
		return survey.Required(selection.Value)
	}

	return survey.Required(answer)
}
