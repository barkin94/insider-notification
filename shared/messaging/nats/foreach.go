package nats

import "context"

// ForEach drains ch until it is closed or ctx is cancelled, calling fn for
// each message. The consumer span is ended automatically after fn returns so
// handlers do not need to manage span lifecycle.
func ForEach[T any](ctx context.Context, ch <-chan Result[T], fn func(Result[T])) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-ch:
			if !ok {
				return
			}
			func() {
				defer result.EndSpan()
				fn(result)
			}()
		}
	}
}
