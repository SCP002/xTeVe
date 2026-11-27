package src

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"unicode"
	"unicode/utf8"
)

// closeOrLog closes c and logs a failure.
//
// Use it for resources that were only read from: a Close error there cannot
// lose data, but it is still worth seeing in the log.
func closeOrLog(c io.Closer) {
	if err := c.Close(); err != nil {
		ShowError(err, 0)
	}
}

// closeKeepErr closes c and stores the Close error in *err, unless *err
// already holds an earlier error.
//
// Use it with defer for files that were written to: a Close error there
// means the written data may be incomplete.
func closeKeepErr(c io.Closer, err *error) {
	if closeErr := c.Close(); closeErr != nil && *err == nil {
		*err = closeErr
	}
}

// removeAllOrLog removes path and everything it contains and logs a failure.
func removeAllOrLog(path string) {
	if err := os.RemoveAll(path); err != nil {
		ShowError(err, 0)
	}
}

// logUnlessNotExist logs err unless it only says that a file does not exist,
// which is an expected answer (HTTP 404) rather than a failure.
func logUnlessNotExist(err error) {
	if !errors.Is(err, fs.ErrNotExist) {
		ShowError(err, 0)
	}
}

// writeResponse writes data to the HTTP client and reports whether it succeeded.
//
// A failure means the client has gone away, which is routine for streaming
// clients and media servers, so it is only shown in debug mode.
func writeResponse(w http.ResponseWriter, data []byte) bool {
	if _, err := w.Write(data); err != nil {
		showDebug(fmt.Sprintf("HTTP Response:%s", err), 1)
		return false
	}
	return true
}

// stopProcess kills an external buffer process (FFmpeg / VLC) and waits for
// it to exit.
func stopProcess(cmd *exec.Cmd) {
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		ShowError(err, 0)
	}

	// A killed process always exits with an *exec.ExitError, so only other
	// errors (such as failed output copying) are worth reporting.
	if err := cmd.Wait(); err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); !ok {
			ShowError(err, 0)
		}
	}
}

// userMessage turns an error into a message for the web UI.
//
// Go error strings start with a lowercase letter, so the first letter is
// capitalized for display.
func userMessage(err error) string {
	var msg = err.Error()
	r, size := utf8.DecodeRuneInString(msg)
	if r == utf8.RuneError {
		return msg
	}
	return string(unicode.ToUpper(r)) + msg[size:]
}

// codedError attaches an xTeVe error code (see getErrMsg) to an error, so that
// the code reaches the caller that finally reports the error with ShowError.
type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }

func (e *codedError) Unwrap() error { return e.err }

// withCode attaches an xTeVe error code to err. It returns nil if err is nil.
func withCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// errorCode returns the xTeVe error code attached to err with withCode, or 0.
func errorCode(err error) int {
	if coded, ok := errors.AsType[*codedError](err); ok {
		return coded.code
	}
	return 0
}
