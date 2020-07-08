package util

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gofrs/uuid"
)

var (
	logFile io.WriteCloser
	logger  *log.Logger
)

func InitFatalLogWriter(path string, stderr bool) error {
	var writers []io.Writer

	if path != "" {
		var err error

		logFile, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("unable to open log error file for appending: %w", err)
		}

		writers = append(writers, logFile)
	}

	if stderr {
		writers = append(writers, os.Stderr)
	}

	if writers != nil {
		logger = log.New(io.MultiWriter(writers...), "phenix", log.LstdFlags)
	}

	return nil
}

func CloseLogWriter() {
	if logFile != nil {
		logFile.Close()
	}
}

func LogErrorGetID(err error) string {
	uuid := uuid.Must(uuid.NewV4()).String()

	if logger != nil {
		logger.Printf("[%s] %v", uuid, err)
	}

	return uuid
}

type HumanizedError struct {
	cause     error
	humanized string
	uuid      string
}

func HumanizeError(err error, desc string) *HumanizedError {
	var h *HumanizedError

	if errors.As(err, &h) {
		return h
	}

	return &HumanizedError{
		cause:     err,
		humanized: desc,
		uuid:      LogErrorGetID(err),
	}
}

func (this HumanizedError) Error() string {
	return this.cause.Error()
}

func (this HumanizedError) Unwrap() error {
	return this.cause
}

func (this HumanizedError) Humanize() string {
	if this.humanized == "" {
		err := strings.Split(this.cause.Error(), " ")
		err[0] = strings.Title(err[0])

		return strings.Join(err, " ")
	}

	return fmt.Sprintf("%s (search error logs for %s)", this.humanized, this.uuid)
}

func (this HumanizedError) UUID() string {
	return this.uuid
}
