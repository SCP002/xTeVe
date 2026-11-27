package src

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"xteve/src/internal/errcode"
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

// userMessage turns an error into a message for the web UI.
//
// Go error strings start with a lowercase letter, so the first letter is
// capitalized for display.
func userMessage(err error) string { return errcode.UserMessage(err) }

// withCode attaches an xTeVe error code (see getErrMsg) to err, see errcode.With.
func withCode(err error, code int) error { return errcode.With(err, code) }

// errorCode returns the xTeVe error code attached to err, see errcode.Of.
func errorCode(err error) int { return errcode.Of(err) }
