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

// ForEachBatch drains ch until it is closed or ctx is cancelled, calling fn
// for each batch. Spans for all messages in the batch are ended after fn
// returns; handlers are responsible for ACK/NAK per message individually.
func ForEachBatch[T any](ctx context.Context, ch <-chan []Result[T], fn func([]Result[T])) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-ch:
			if !ok {
				return
			}
			func() {
				defer func() {
					for _, r := range batch {
						r.EndSpan()
					}
				}()
				fn(batch)
			}()
		}
	}
}
