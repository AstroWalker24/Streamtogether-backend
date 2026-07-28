package logger 

import (
	"time"
	"github.com/rs/zerolog"
)

type Field struct {
    apply func(e *zerolog.Event) *zerolog.Event
    ctx   func(c zerolog.Context) zerolog.Context
}

func String(key, val string) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Str(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Str(key, val) },
    }
}

func Int(key string, val int) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Int(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Int(key, val) },
    }
}

func Int64(key string, val int64) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Int64(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Int64(key, val) },
    }
}

func Bool(key string, val bool) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Bool(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Bool(key, val) },
    }
}

func Duration(key string, val time.Duration) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Dur(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Dur(key, val) },
    }
}

func Time(key string, val time.Time) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Time(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Time(key, val) },
    }
}

func Any(key string, val interface{}) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Interface(key, val) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Interface(key, val) },
    }
}

func Err(err error) Field {
    return Field{
        apply: func(e *zerolog.Event) *zerolog.Event { return e.Err(err) },
        ctx:   func(c zerolog.Context) zerolog.Context { return c.Err(err) },
    }
}
