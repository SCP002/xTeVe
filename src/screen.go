package src

import (
	"fmt"

	"xteve/src/internal/errcode"
	"xteve/src/internal/logger"
)

func showInfo(str string) { logger.Info(str) }

func showDebug(str string, level int) { logger.Debug(level, str) }

func showHighlight(str string) { logger.Highlight(str) }

func showWarning(errCode int) { errcode.Warn(errCode) }

// ShowError : Shows the Error Messages in the Console. A code that comes with the error itself (see withCode) is more
// specific than errCode.
func ShowError(err error, errCode int) { errcode.Report(err, errCode) }

// getErrMsg returns the message of an xTeVe error code.
func getErrMsg(errCode int) string { return errcode.Message(errCode) }

func sendAlert(text string) {
	select {
	case webAlerts <- text:
		//
	default:
		err := fmt.Errorf("client alert buffer is full, dropping the message: %v", text)
		ShowError(err, 0)
	}
}
