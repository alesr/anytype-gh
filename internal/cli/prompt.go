package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

const selectFilteringThreshold = 10

var (
	errPromptValueRequired = errors.New("a value is required")
	errNoOptionsAvailable  = errors.New("no options available")
)

type prompter struct {
	in            io.Reader
	out           io.Writer
	useAccessible bool
}

func newPrompter(input io.Reader, output io.Writer) *prompter {
	return &prompter{
		in:            input,
		out:           output,
		useAccessible: !isInteractiveTTY(input, output),
	}
}

func (p *prompter) readLine(label string) (string, error) {
	var value string

	field := huh.NewInput().
		Title(strings.TrimSpace(label)).
		Validate(
			func(input string) error {
				if strings.TrimSpace(input) == "" {
					return errPromptValueRequired
				}
				return nil
			},
		).
		Value(&value)

	if err := p.runInput(field); err != nil {
		return "", fmt.Errorf("could not read line: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func (p *prompter) runInput(field *huh.Input) error {
	if p.useAccessible {
		return field.RunAccessible(p.out, p.in)
	}
	return field.Run()
}

func (p *prompter) runSelect(field *huh.Select[int]) error {
	if p.useAccessible {
		return field.RunAccessible(p.out, p.in)
	}
	return field.Run()
}

func isInteractiveTTY(input io.Reader, output io.Writer) bool {
	inFile, inOK := input.(*os.File)
	outFile, outOK := output.(*os.File)

	if !inOK || !outOK {
		return false
	}
	return term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func mapOptions(options []string) []huh.Option[int] {
	mapped := make([]huh.Option[int], 0, len(options))
	for idx, option := range options {
		mapped = append(mapped, huh.NewOption(option, idx))
	}
	return mapped
}

func (p *prompter) chooseIndex(label string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, errNoOptionsAvailable
	}
	filterEnabled := len(options) >= selectFilteringThreshold

	var selected int

	field := huh.NewSelect[int]().
		Title(strings.TrimSpace(label)).
		Description(selectDescription(filterEnabled)).
		Filtering(filterEnabled).
		Options(mapOptions(options)...).
		Value(&selected)

	if err := p.runSelect(field); err != nil {
		return -1, err
	}
	return selected, nil
}

func selectDescription(filterEnabled bool) string {
	if filterEnabled {
		return "Use arrows and Enter. Press / to filter."
	}
	return "Use arrows and Enter."
}
