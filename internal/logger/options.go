package logger 

import (
	"io"
	"os"
	"time"
	"github.com/rs/zerolog"
)

type Options struct {
    level      zerolog.Level
    pretty     bool
    writer     io.Writer
    caller     bool
    timeFormat string
}

type Option func(*Options)

func WithLevel(level string) Option {
    return func(o *Options) {
        o.level = parseLevel(level)
    }
}

func WithPretty(enabled bool) Option {
    return func(o *Options) {
        o.pretty = enabled
    }
}

func WithWriter(w io.Writer) Option {
    return func(o *Options) {
        o.writer = w
    }
}

func WithCaller() Option {
    return func(o *Options) {
        o.caller = true
    }
}

func WithTimeFormat(format string) Option {
    return func(o *Options) {
        o.timeFormat = format
    }
}

func defaults() Options {
    return Options{
        level:      zerolog.InfoLevel,
        pretty:     false,
        writer:     os.Stdout,
        caller:     false,
        timeFormat: time.RFC3339,
    }
}

func parseLevel(level string) zerolog.Level {
    switch level {
    case "debug":
        return zerolog.DebugLevel
    case "warn":
        return zerolog.WarnLevel
    case "error":
        return zerolog.ErrorLevel
    case "fatal":
        return zerolog.FatalLevel
    default:
        return zerolog.InfoLevel
    }
}