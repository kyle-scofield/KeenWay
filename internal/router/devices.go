package router

import (
	"regexp"
	"strings"
)

// Device is a host the router knows about.
type Device struct {
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	Active     bool   `json:"active"`
	Registered bool   `json:"registered"`
	RX         int64  `json:"rx"` // bytes the router counted for this device
	TX         int64  `json:"tx"`
}

// Label is the best human name for the device.
func (d Device) Label() string {
	switch {
	case d.Name != "":
		return d.Name
	case d.Hostname != "":
		return d.Hostname
	default:
		return d.MAC
	}
}

var kvLine = regexp.MustCompile(`^(\s*)([a-z-]+):\s*(.*)$`)

// ParseHotspot reads "show ip hotspot" output: one block per host, each starting with "mac:".
// The CLI right-aligns keys, so fields of the same level share the colon's column.
func ParseHotspot(out string) []Device {
	var devs []Device
	var cur *Device
	hostColon := -1
	for _, line := range strings.Split(out, "\n") {
		m := kvLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		colon, k, v := len(m[1])+len(m[2]), m[2], strings.TrimSpace(m[3])
		if k == "mac" {
			devs = append(devs, Device{MAC: strings.ToLower(v)})
			cur = &devs[len(devs)-1]
			hostColon = colon
			continue
		}
		// Nested blocks (e.g. "interface: name: Home") sit at a different column.
		if cur == nil || colon != hostColon {
			continue
		}
		switch k {
		case "ip":
			if v != "0.0.0.0" {
				cur.IP = v
			}
		case "hostname":
			cur.Hostname = v
		case "name":
			cur.Name = v
		case "active":
			cur.Active = v == "yes"
		case "registered":
			cur.Registered = v == "yes"
		}
	}
	return devs
}

var macRe = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)
