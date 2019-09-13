package sftpd

import (
	"os"
	"time"

	"github.com/drakkan/sftpgo/dataprovider"
	"github.com/drakkan/sftpgo/logger"
	"github.com/drakkan/sftpgo/metrics"
)

const (
	transferUpload = iota
	transferDownload
)

const (
	uploadModeStandard = iota
	uploadModeAtomic
)

// Transfer contains the transfer details for an upload or a download.
// It implements the io Reader and Writer interface to handle files downloads and uploads
type Transfer struct {
	file          *os.File
	path          string
	start         time.Time
	bytesSent     int64
	bytesReceived int64
	user          dataprovider.User
	connectionID  string
	transferType  int
	lastActivity  time.Time
	isNewFile     bool
	protocol      string
	transferError error
	isFinished    bool
}

// TransferError is called if there is an unexpected error.
// For example network or client issues
func (t *Transfer) TransferError(err error) {
	t.transferError = err
	elapsed := time.Since(t.start).Nanoseconds() / 1000000
	logger.Warn(logSender, t.connectionID, "Unexpected error for transfer, path: %#v, error: \"%v\" bytes sent: %v, "+
		"bytes received: %v transfer running since %v ms", t.path, t.transferError, t.bytesSent, t.bytesReceived, elapsed)
}

// ReadAt reads len(p) bytes from the File to download starting at byte offset off and updates the bytes sent.
// It handles download bandwidth throttling too
func (t *Transfer) ReadAt(p []byte, off int64) (n int, err error) {
	t.lastActivity = time.Now()
	readed, e := t.file.ReadAt(p, off)
	t.bytesSent += int64(readed)
	t.handleThrottle()
	return readed, e
}

// WriteAt writes len(p) bytes to the uploaded file starting at byte offset off and updates the bytes received.
// It handles upload bandwidth throttling too
func (t *Transfer) WriteAt(p []byte, off int64) (n int, err error) {
	t.lastActivity = time.Now()
	written, e := t.file.WriteAt(p, off)
	t.bytesReceived += int64(written)
	t.handleThrottle()
	return written, e
}

// Close it is called when the transfer is completed.
// It closes the underlying file, log the transfer info, update the user quota (for uploads)
// and execute any defined actions.
// If there is an error no action will be executed and, in atomic mode, we try to delete
// the temporary file
func (t *Transfer) Close() error {
	err := t.file.Close()
	if t.isFinished {
		return err
	}
	t.isFinished = true
	numFiles := 0
	if t.isNewFile {
		numFiles = 1
	}
	if t.transferType == transferUpload && t.file.Name() != t.path {
		if t.transferError == nil {
			err = os.Rename(t.file.Name(), t.path)
			logger.Debug(logSender, t.connectionID, "atomic upload completed, rename: %#v -> %#v, error: %v",
				t.file.Name(), t.path, err)
		} else {
			err = os.Remove(t.file.Name())
			logger.Warn(logSender, t.connectionID, "atomic upload completed with error: \"%v\", delete temporary file: %#v, "+
				"deletion error: %v", t.transferError, t.file.Name(), err)
			if err == nil {
				numFiles--
				t.bytesReceived = 0
			}
		}
	}
	if t.transferError == nil {
		elapsed := time.Since(t.start).Nanoseconds() / 1000000
		if t.transferType == transferDownload {
			logger.TransferLog(downloadLogSender, t.path, elapsed, t.bytesSent, t.user.Username, t.connectionID, t.protocol)
			executeAction(operationDownload, t.user.Username, t.path, "")
		} else {
			logger.TransferLog(uploadLogSender, t.path, elapsed, t.bytesReceived, t.user.Username, t.connectionID, t.protocol)
			executeAction(operationUpload, t.user.Username, t.path, "")
		}
	}
	metrics.TransferCompleted(t.bytesSent, t.bytesReceived, t.transferType, t.transferError)
	removeTransfer(t)
	if t.transferType == transferUpload && (numFiles != 0 || t.bytesReceived > 0) {
		dataprovider.UpdateUserQuota(dataProvider, t.user, numFiles, t.bytesReceived, false)
	}
	return err
}

func (t *Transfer) handleThrottle() {
	var wantedBandwidth int64
	var trasferredBytes int64
	if t.transferType == transferDownload {
		wantedBandwidth = t.user.DownloadBandwidth
		trasferredBytes = t.bytesSent
	} else {
		wantedBandwidth = t.user.UploadBandwidth
		trasferredBytes = t.bytesReceived
	}
	if wantedBandwidth > 0 {
		// real and wanted elapsed as milliseconds, bytes as kilobytes
		realElapsed := time.Since(t.start).Nanoseconds() / 1000000
		// trasferredBytes / 1000 = KB/s, we multiply for 1000 to get milliseconds
		wantedElapsed := 1000 * (trasferredBytes / 1000) / wantedBandwidth
		if wantedElapsed > realElapsed {
			toSleep := time.Duration(wantedElapsed - realElapsed)
			time.Sleep(toSleep * time.Millisecond)
		}
	}
}
