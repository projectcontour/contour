package fixture

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

// NewTestLogger returns logrus.Logger that writes messages using (*testing.T)Logf.
func NewTestLogger(t *testing.T) *logrus.Logger {
	log := logrus.New()
	log.Out = &testWriter{t}
	return log
}

type discardWriter struct{}

func (d *discardWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}

// NewDiscardLogger returns logrus.Logger that discards log messages.
func NewDiscardLogger() *logrus.Logger {
	log := logrus.New()
	log.Out = &discardWriter{}
	return log
}
