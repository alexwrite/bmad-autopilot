package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexwrite/bmad-autopilot/internal/orchestrator"
)

// installSignalStop wires SIGINT/SIGTERM to a two-stage shutdown suited to an
// unattended headless run:
//
//   - first signal  → graceful stop: the runner finishes the current step
//     (so the working tree and run.log stay consistent), then exits clean.
//   - second signal → hard abort: cancel the context, killing the in-flight
//     `claude` subprocess immediately.
//
// The returned cleanup func detaches the handler and stops the goroutine.
func installSignalStop(stop *orchestrator.StopController, cancel context.CancelFunc, out io.Writer) func() {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		hits := 0
		for {
			select {
			case <-done:
				return
			case <-ch:
				hits++
				if hits == 1 {
					stop.RequestStop()
					fmt.Fprintln(out, "STOP: graceful stop requested — finishing the current step, then exiting. Signal again to abort now.")
					continue
				}
				fmt.Fprintln(out, "ABORT: cancelling the in-flight command now.")
				cancel()
				return
			}
		}
	}()

	return func() {
		signal.Stop(ch)
		close(done)
	}
}
