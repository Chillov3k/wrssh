//go:build execass

package execass

import "github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"

func init() {
	subsystems.Register(New())
}
