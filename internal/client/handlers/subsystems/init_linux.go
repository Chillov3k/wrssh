package subsystems

// Enable setuid and setgid for linux only
func init() {
	Register(newSetuidModule())
	Register(newSetgidModule())
	Register(newServiceModule())
}
