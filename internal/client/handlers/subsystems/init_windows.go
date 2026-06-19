//go:build windows

package subsystems

// Enable service install
func init() {
	Register(newServiceModule())
}
