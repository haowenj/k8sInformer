package log

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
)

// NewStdoutLogger returns a logr instance that prints log into os.Stdout.
func NewStdoutLogger() logr.Logger {
	return funcr.New(func(prefix, args string) {
		if prefix != "" {
			fmt.Printf("%s: %s\n", prefix, args)
		} else {
			fmt.Printf("%s\n", args)
		}
	}, funcr.Options{LogCallerFunc: true, LogTimestamp: true, TimestampFormat: time.DateTime})
}
