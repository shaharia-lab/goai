package mcp

import "fmt"

// Version represents the server version
type Version struct {
	Major int
	Minor int
	Patch int
}

// String returns the version as a string
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
