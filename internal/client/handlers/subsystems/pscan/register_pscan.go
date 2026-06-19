//go:build pscan

package pscan

import "github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"

func init() {
	subsystems.Register(New())
}
