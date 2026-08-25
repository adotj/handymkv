package hmkv

import (
	"errors"
	"fmt"
)

var ErrInvalidInput = errors.New("invalid input")

type DiscError struct {
	DiscId int
	Msg    string
}

func (e *DiscError) Error() string {
	return fmt.Sprintf("disc %d: %s", e.DiscId, e.Msg)
}

func NewDiscError(discId int, msg string) error {
	return &DiscError{
		DiscId: discId,
		Msg:    msg,
	}
}

var ErrTitlesDiscRead = errors.New("cannot read titles from disc")

type ExternalProcessError struct {
	Err           error
	ProcessOutput string
}

func NewExternalProcessError(err error, output string) *ExternalProcessError {
	return &ExternalProcessError{
		Err:           err,
		ProcessOutput: output,
	}
}

func (e *ExternalProcessError) Error() string {
	return e.Err.Error()
}
