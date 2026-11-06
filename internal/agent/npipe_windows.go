package agent

import (
	"context"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
)

func dialPipe(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}

// pipeExists reports whether Docker Desktop's engine pipe is there,
// without connecting to it.
func pipeExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
