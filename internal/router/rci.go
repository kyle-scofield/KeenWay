package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
)

// The router's internal API (RCI) answers on 127.0.0.1:79 without a password,
// so these work from the Entware shell both locally and over SSH, no Keenetic CLI needed.
const rci = "http://127.0.0.1:79/rci"

func (c *Conn) rciGet(ctx context.Context, path string, out any) error {
	text, err := c.Shell(ctx, "curl -s -m 10 "+shellQuote(rci+path))
	if err != nil {
		return err
	}
	if i := strings.IndexAny(text, "{["); i > 0 {
		text = text[i:]
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("неожиданный ответ API роутера")
	}
	return nil
}

// rciCommands runs Keenetic CLI commands through RCI's "parse" call.
func (c *Conn) rciCommands(ctx context.Context, cmds ...string) error {
	var batch []map[string]string
	for _, cmd := range cmds {
		batch = append(batch, map[string]string{"parse": cmd})
	}
	body, _ := json.Marshal(batch)
	out, err := c.Shell(ctx, "curl -s -m 15 -H 'Content-Type: application/json' -d "+shellQuote(string(body))+" "+shellQuote(rci+"/"))
	if err != nil {
		return err
	}
	if strings.Contains(out, `"error"`) {
		return fmt.Errorf("роутер отклонил команду: %s", lastLines(out, 2))
	}
	return nil
}

// Devices lists hosts known to the router, with traffic counters.
func (c *Conn) Devices(ctx context.Context) ([]Device, error) {
	var resp struct {
		Host []struct {
			MAC        string `json:"mac"`
			IP         string `json:"ip"`
			Hostname   string `json:"hostname"`
			Name       string `json:"name"`
			Active     bool   `json:"active"`
			Registered bool   `json:"registered"`
			RX         int64  `json:"rxbytes"`
			TX         int64  `json:"txbytes"`
		} `json:"host"`
	}
	if err := c.rciGet(ctx, "/show/ip/hotspot", &resp); err != nil {
		return nil, err
	}
	devs := make([]Device, 0, len(resp.Host))
	for _, h := range resp.Host {
		d := Device{MAC: strings.ToLower(h.MAC), Name: h.Name, Hostname: h.Hostname, Active: h.Active, Registered: h.Registered, RX: h.RX, TX: h.TX}
		if h.IP != "0.0.0.0" {
			d.IP = h.IP
		}
		devs = append(devs, d)
	}
	return devs, nil
}

// PinIP reserves an IP for a device in the router's DHCP server so per-device rules keep working.
func (c *Conn) PinIP(ctx context.Context, mac, ip string) error {
	mac = strings.ToLower(mac)
	addr, err := netip.ParseAddr(ip)
	if !macRe.MatchString(mac) || err != nil || !addr.Is4() {
		return fmt.Errorf("неверный MAC или IP")
	}
	return c.rciCommands(ctx, "ip dhcp host "+mac+" "+addr.String(), "system configuration save")
}

// MAC returns the router's LAN MAC (providers often use it as the device ID, "x-hwid").
func (c *Conn) MAC(ctx context.Context) (string, error) {
	out, err := c.Shell(ctx, "cat /sys/class/net/br0/address")
	if err != nil {
		return "", err
	}
	mac := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(lastLines(out, 1)), ":", ""))
	if len(mac) != 12 {
		return "", fmt.Errorf("MAC роутера не найден")
	}
	return mac, nil
}

// LANAddress returns the router's own IP in the home network.
func (c *Conn) LANAddress(ctx context.Context) (string, error) {
	out, err := c.Shell(ctx, "ip -4 -o addr show br0 | awk '{print $4}' | cut -d/ -f1 | head -1")
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(lastLines(out, 1))
	if _, err := netip.ParseAddr(ip); err != nil {
		return "", fmt.Errorf("IP роутера не найден")
	}
	return ip, nil
}
