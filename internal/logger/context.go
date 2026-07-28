package logger 

import "context"


type contextKey struct{}

func WithContext(ctx context.Context, l Logger) context.Context {
    return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) Logger {
    if l, ok := ctx.Value(contextKey{}).(Logger); ok {
        return l
    }
    return Nop()
}