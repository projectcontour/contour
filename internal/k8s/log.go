package k8s

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/klog"
)

type klogParams struct {
	flags *flag.FlagSet
	log   *logrus.Entry
}

type LogOption func(*klogParams)

// LogLevelOption creates an option to set the Kubernetes verbose
// log level (1 - 10 is the standard range).
func LogLevelOption(level int) LogOption {
	return LogOption(func(p *klogParams) {
		if err := p.flags.Set("v", strconv.Itoa(level)); err != nil {
			panic(fmt.Sprintf("failed to set flag: %s", err))
		}
	})
}

// LogWriterOption creates an option to set the Kubernetes logging output.
func LogWriterOption(log *logrus.Entry) LogOption {
	return LogOption(func(p *klogParams) {
		p.log = log
	})
}

// InitLogging initializes the Kubernetes client-go logging subsystem.
func InitLogging(options ...LogOption) {
	must := func(err error) {
		if err != nil {
			panic(err.Error())
		}
	}

	p := klogParams{
		flags: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}

	// First, init the flags so that we can set specific values.
	klog.InitFlags(p.flags)

	for _, o := range options {
		o(&p)
	}

	switch p.log {
	case nil:
	default:
		// Force klog to a file output so that it uses the PipeWriter.
		must(p.flags.Set("logtostderr", "false"))
		must(p.flags.Set("alsologtostderr", "false"))

		klog.SetOutput(makeWriter(p.log))
	}
}

// makeWriter is based on (*Entry).Writer() from logrus, but has a
// couple of improvements. We avoid the fixed buffer size of using
// bufio.Scanner, and we take advantage of out knowledge of the fixed
// klog output format to improve the final logging output.
func makeWriter(entry *logrus.Entry) io.Writer {
	closer := func(writer *io.PipeWriter) {
		writer.Close()
	}

	pipeReader, pipeWriter := io.Pipe()
	runtime.SetFinalizer(pipeWriter, closer)

	go func() {
		defer pipeReader.Close()

		reader := bufio.NewReader(pipeReader)

		for {
			line, err := reader.ReadString('\n')
			switch err {
			case nil:
			case io.EOF:
				// Most likely, when the pipe closes, klog is being reinitialized.
				return
			default:
				entry.Errorf("error reading from log pipe: %s", err)
				return
			}

			// klog logs have the following format: Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...
			fields := strings.SplitN(line, "] ", 2)

			// Split out the file location. I could not
			// find a reasonable way to make this work
			// with (*Logger).SetReportCaller(), so we
			// preserve it in the "location" field.
			location := fields[0][strings.LastIndexByte(fields[0], ' ')+1:]

			e := entry.WithField("location", location)

			// The first character of the first header field is the log level.
			switch fields[0][0] {
			case 'I':
				e.Info(fields[1])
			case 'W':
				e.Warn(fields[1])
			case 'E':
				e.Error(fields[1])
			case 'F':
				e.Fatal(fields[1])
			}
		}
	}()

	return pipeWriter
}
