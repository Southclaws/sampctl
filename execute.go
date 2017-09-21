package main

// Execute handles the actual running of the server process - it collects log output too
func Execute(endpoint, version, dir string) (err error) {
	// check if matching binary exists in dir using md5
	// if not, run GetPackage
	// create a Server object from config/env vars
	// generate server.cfg
	// execute platform binary in goroutine
	// collect stdout and print in other goroutine
	// in future, add options to send to syslog, database or something
	// also, parse log depending on the log_format setting, pull out time/date and maybe in future, allow custom regex for grouping outputs
	return
}
