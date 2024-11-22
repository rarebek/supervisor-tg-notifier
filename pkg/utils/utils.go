package utils

import (
	"crypto/md5"
	"fmt"
	"strings"
)

func GetShortServerId(serverURL string) string {
	// Extract just the host and port from the URL
	parts := strings.Split(serverURL, "://")
	if len(parts) > 1 {
		hostPort := strings.Split(parts[1], "/")[0]
		hash := fmt.Sprintf("%x", md5.Sum([]byte(hostPort)))
		return hash[:8]
	}
	hash := fmt.Sprintf("%x", md5.Sum([]byte(serverURL)))
	return hash[:8]
}
