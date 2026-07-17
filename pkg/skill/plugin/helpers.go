package plugin

import (
	"bytes"
	"io"
	"strings"

	"github.com/tickraft/taichi/pkg/skill"
)

// bytesReader wraps a byte slice as an io.Reader, used for cmd.Stdin.
func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

// logWriter forwards the plugin process's stderr output line by line to the taichi logger.
type logWriter struct {
	logger    skill.Logger
	skillName string
	buf       bytes.Buffer
}

// newLogWriter creates a writer that forwards stderr to the logger.
func newLogWriter(logger skill.Logger, skillName string) *logWriter {
	return &logWriter{logger: logger, skillName: skillName}
}

// Write implements io.Writer. It splits input by line and forwards each line to the logger.
func (w *logWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		s := w.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(s[:idx], "\r")
		// Preserve any unconsumed remainder.
		w.buf.Reset()
		if idx+1 < len(s) {
			w.buf.WriteString(s[idx+1:])
		}
		if line != "" {
			w.logger.Infof("[plugin:%s] %s", w.skillName, line)
		}
	}
	return len(p), nil
}
