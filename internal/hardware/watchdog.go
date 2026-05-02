package hardware

import (
	"context"
	"time"
)

func StartWatchdog(ctx context.Context, interval time.Duration, limitMB int, probe func() ProbeResult, onPressure func(ProbeResult)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := probe()
				if state.AvailableMemoryMB < limitMB {
					onPressure(state)
				}
			}
		}
	}()
	return done
}
