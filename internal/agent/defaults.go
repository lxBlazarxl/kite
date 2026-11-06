package agent

// defaultNetExclude lists interface name globs hidden by default: virtual
// adapters that only add noise to the dashboard.
func defaultNetExclude(goos string) string {
	if goos == "windows" {
		return "Loopback*,Local Area Connection*,vEthernet*,isatap*,Teredo*,Bluetooth*"
	}
	return "lo,veth*,br-*,docker0,virbr*"
}

// defaultDockerHost is where the Docker Engine API listens by default.
func defaultDockerHost(goos string) string {
	if goos == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}
