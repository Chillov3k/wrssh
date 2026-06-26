package subsystems

// Enable core subsystems for every supported client build.
func init() {
	Register(newSFTPModule())
	Register(newListModule())
}
