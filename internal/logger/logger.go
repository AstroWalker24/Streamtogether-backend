package logger

import (
	"github.com/rs/zerolog"
)

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
}

type impl struct {
	zl zerolog.Logger
}

func New(opts ...Option) Logger {
	o := defaults()
	for _, opt := range opts {
		opt(&o)
	}

	zerolog.TimeFieldFormat = o.timeFormat

	out := o.writer
	if o.pretty {
		out = zerolog.ConsoleWriter{Out: o.writer, TimeFormat: o.timeFormat}
	}

	zl := zerolog.New(out).
		Level(o.level).
		With().
		Timestamp().
		Logger()

	if o.caller {
		zl = zl.With().Caller().Logger()
	}

	return &impl{zl: zl}
}

func Nop() Logger {
	return &impl{zl: zerolog.Nop()}
}

func (l *impl) Debug(msg string, fields ...Field) {
	l.emit(l.zl.Debug(), msg, fields)
}

func (l *impl) Info(msg string, fields ...Field) {
	l.emit(l.zl.Info(), msg, fields)
}

func (l *impl) Warn(msg string, fields ...Field) {
	l.emit(l.zl.Warn(), msg, fields)
}

func (l *impl) Error(msg string, fields ...Field) {
	l.emit(l.zl.Error(), msg, fields)
}

func (l *impl) Fatal(msg string, fields ...Field) {
	l.emit(l.zl.Fatal(), msg, fields)
}

func (l *impl) With(fields ...Field) Logger {
	ctx := l.zl.With()
	for _, f := range fields {
		ctx = f.ctx(ctx)
	}
	return &impl{zl: ctx.Logger()}
}

func (l *impl) emit(event *zerolog.Event, msg string, fields []Field) {
	if event == nil {
		return
	}
	for _, f := range fields {
		event = f.apply(event)
	}
	event.Msg(msg)
}
