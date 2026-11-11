// Package mihomo is a small client for the mihomo REST API (the same one zashboard uses).
package mihomo

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client talks to http://<router>:9090.
type Client struct {
	Base   string
	Secret string
	HTTP   *http.Client
}

func New(host, secret string) *Client {
	return &Client{Base: "http://" + host + ":9090", Secret: secret, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// ErrUnauthorized means the panel password is wrong or missing.
var ErrUnauthorized = fmt.Errorf("неверный пароль панели mihomo")

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, r)
	if err != nil {
		return err
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("панель mihomo недоступна: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("mihomo: %s %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Version returns the running mihomo version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	err := c.do(ctx, http.MethodGet, "/version", nil, &v)
	return v.Version, err
}

type delayHistory struct {
	Delay int `json:"delay"`
}

type proxyInfo struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Now     string         `json:"now"`
	All     []string       `json:"all"`
	Hidden  bool           `json:"hidden"`
	Icon    string         `json:"icon"`
	History []delayHistory `json:"history"`
	Alive   *bool          `json:"alive"`
}

// Group is a proxy group as shown in the app (and in zashboard).
type Group struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	Options []string `json:"options"`
	Icon    string   `json:"icon,omitempty"`
	Hidden  bool     `json:"hidden"`
}

// Node is one server with its last measured delay (0 = unknown or dead).
type Node struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Delay    int    `json:"delay"`
	Provider string `json:"provider,omitempty"`
}

// Groups lists selection groups with the currently chosen server.
func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var resp struct {
		Proxies map[string]proxyInfo `json:"proxies"`
	}
	if err := c.do(ctx, http.MethodGet, "/proxies", nil, &resp); err != nil {
		return nil, err
	}
	// mihomo keeps the configured group order in the GLOBAL group.
	order := resp.Proxies["GLOBAL"].All
	var groups []Group
	for _, name := range order {
		p, ok := resp.Proxies[name]
		if !ok || len(p.All) == 0 {
			continue
		}
		groups = append(groups, Group{Name: p.Name, Type: p.Type, Now: p.Now, Options: p.All, Icon: p.Icon, Hidden: p.Hidden})
	}
	return groups, nil
}

// Select switches a select-type group to the given option.
func (c *Client) Select(ctx context.Context, group, option string) error {
	return c.do(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": option}, nil)
}

// SubscriptionInfo is what providers report about quota and expiry (shown like in Happ).
type SubscriptionInfo struct {
	Upload   int64 `json:"Upload"`
	Download int64 `json:"Download"`
	Total    int64 `json:"Total"`
	Expire   int64 `json:"Expire"`
}

// Provider is a subscription as mihomo sees it on the router.
type Provider struct {
	Name      string            `json:"name"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Info      *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
	Nodes     []Node            `json:"nodes"`
	Alive     int               `json:"alive"`
}

// Providers lists subscriptions with their servers and quota.
func (c *Client) Providers(ctx context.Context) ([]Provider, error) {
	var resp struct {
		Providers map[string]struct {
			Name             string            `json:"name"`
			VehicleType      string            `json:"vehicleType"`
			UpdatedAt        time.Time         `json:"updatedAt"`
			SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo"`
			Proxies          []proxyInfo       `json:"proxies"`
		} `json:"providers"`
	}
	if err := c.do(ctx, http.MethodGet, "/providers/proxies", nil, &resp); err != nil {
		return nil, err
	}
	var out []Provider
	for name, p := range resp.Providers {
		if p.VehicleType == "Compatible" || name == "default" {
			continue // mihomo's built-in provider for proxies listed directly in the config
		}
		pr := Provider{Name: name, UpdatedAt: p.UpdatedAt, Info: p.SubscriptionInfo}
		for _, n := range p.Proxies {
			node := Node{Name: n.Name, Type: n.Type, Provider: name}
			if len(n.History) > 0 {
				node.Delay = n.History[len(n.History)-1].Delay
			}
			if node.Delay > 0 {
				pr.Alive++
			}
			pr.Nodes = append(pr.Nodes, node)
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UpdateProvider re-downloads a subscription right now.
func (c *Client) UpdateProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPut, "/providers/proxies/"+url.PathEscape(name), nil, nil)
}

// Plain HTTP keeps the test cheap: a Keenetic CPU cannot finish ~100 TLS handshakes at once
// within the timeout, and every server then looks dead.
const testURL = "http://www.gstatic.com/generate_204"

// TestGroup measures every server in a group; the result maps server name to delay in ms (0 = failed).
func (c *Client) TestGroup(ctx context.Context, group string) (map[string]int, error) {
	out := map[string]int{}
	err := c.do(ctx, http.MethodGet, "/group/"+url.PathEscape(group)+"/delay?timeout=8000&url="+url.QueryEscape(testURL), nil, &out)
	if err != nil && strings.Contains(err.Error(), "all proxies timeout") {
		return out, nil // nothing answered: every server shows as failed
	}
	return out, err
}

// Delay measures one server or group.
func (c *Client) Delay(ctx context.Context, name string) (int, error) {
	var d struct {
		Delay int `json:"delay"`
	}
	err := c.do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?timeout=5000&url="+url.QueryEscape(testURL), nil, &d)
	return d.Delay, err
}

// Connection is one open connection through mihomo.
type Connection struct {
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Chains   []string `json:"chains"`
	Rule     string   `json:"rule"`
	Payload  string   `json:"rulePayload"`
	Metadata struct {
		SourcePort string `json:"sourcePort"`
		SourceIP   string `json:"sourceIP"`
		Host       string `json:"host"`
		DestIP     string `json:"destinationIP"`
	} `json:"metadata"`
}

// Snapshot is the live traffic picture.
type Snapshot struct {
	UploadTotal   int64        `json:"uploadTotal"`
	DownloadTotal int64        `json:"downloadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        int64        `json:"memory"`
}

// Traffic returns totals since mihomo started plus the open connections.
func (c *Client) Traffic(ctx context.Context) (*Snapshot, error) {
	var s Snapshot
	err := c.do(ctx, http.MethodGet, "/connections", nil, &s)
	return &s, err
}

// Usage is traffic grouped by a key (device IP or service).
type Usage struct {
	Key         string `json:"key"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Connections int    `json:"connections"`
	ViaVPN      int64  `json:"viaVPN"`
}

// ByDevice and ByService summarise the open connections.
func (s *Snapshot) ByDevice() []Usage {
	return s.group(func(c Connection) string { return c.Metadata.SourceIP })
}

func (s *Snapshot) ByService() []Usage {
	return s.group(func(c Connection) string {
		if len(c.Chains) > 0 {
			return c.Chains[len(c.Chains)-1] // the group that matched, e.g. "YouTube"
		}
		return c.Rule
	})
}

func (s *Snapshot) group(key func(Connection) string) []Usage {
	m := map[string]*Usage{}
	for _, c := range s.Connections {
		k := key(c)
		u := m[k]
		if u == nil {
			u = &Usage{Key: k}
			m[k] = u
		}
		u.Upload += c.Upload
		u.Download += c.Download
		u.Connections++
		if len(c.Chains) > 0 && c.Chains[0] != "DIRECT" && c.Chains[0] != "REJECT" {
			u.ViaVPN += c.Upload + c.Download
		}
	}
	out := make([]Usage, 0, len(m))
	for _, u := range m {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Upload+out[i].Download > out[j].Upload+out[j].Download })
	return out
}

// MixedPort is the HTTP/SOCKS port of the generated config.
const MixedPort = 1080

// Route tells which rule and which servers mihomo uses for a site. It opens a
// CONNECT through the router's mixed port and reads the matching connection back.
type Route struct {
	Host    string   `json:"host"`
	Rule    string   `json:"rule"`
	Payload string   `json:"payload"`
	Chain   []string `json:"chain"` // from the rule's group to the final server
	Opened  bool     `json:"opened"`
}

func (c *Client) Route(ctx context.Context, host string, port int) (*Route, error) {
	u, err := url.Parse(c.Base)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), strconv.Itoa(MixedPort)))
	if err != nil {
		return nil, fmt.Errorf("порт %d роутера недоступен: %v", MixedPort, err)
	}
	defer conn.Close()
	target := net.JoinHostPort(host, strconv.Itoa(port))
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	conn.SetReadDeadline(time.Now().Add(12 * time.Second))
	status, _ := bufio.NewReader(conn).ReadString('\n')
	opened := strings.Contains(status, " 200")
	localPort := strconv.Itoa(conn.LocalAddr().(*net.TCPAddr).Port)

	for try := 0; try < 4; try++ {
		snap, err := c.Traffic(ctx)
		if err != nil {
			return nil, err
		}
		for _, cn := range snap.Connections {
			if cn.Metadata.SourcePort == localPort && strings.EqualFold(cn.Metadata.Host, host) || cn.Metadata.SourcePort == localPort && cn.Metadata.DestIP == host {
				chain := make([]string, 0, len(cn.Chains))
				for i := len(cn.Chains) - 1; i >= 0; i-- {
					chain = append(chain, cn.Chains[i])
				}
				return &Route{Host: host, Rule: cn.Rule, Payload: cn.Payload, Chain: chain, Opened: opened}, nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !opened {
		return nil, fmt.Errorf("mihomo не открыл соединение: %s", strings.TrimSpace(status))
	}
	return nil, fmt.Errorf("соединение не найдено в списке mihomo")
}

// LogLine is one entry of mihomo's live log.
type LogLine struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // debug, info, warning, error
	Message string    `json:"message"`
}

// Logs streams mihomo's live log (the /logs endpoint) until ctx ends. level is the lowest
// level to receive. mihomo sends log events to API clients even with log-level: silent.
func (c *Client) Logs(ctx context.Context, level string, fn func(LogLine)) error {
	switch level {
	case "debug", "info", "warning", "error":
	default:
		level = "info"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/logs?format=structured&level="+level, nil)
	if err != nil {
		return err
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}
	resp, err := (&http.Client{}).Do(req) // no timeout: it is a stream
	if err != nil {
		return fmt.Errorf("панель mihomo недоступна: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mihomo: %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var raw struct {
			Type    string `json:"type"`    // plain format
			Payload string `json:"payload"` // plain format
			Time    string `json:"time"`    // structured format
			Level   string `json:"level"`
			Message string `json:"message"`
			Fields  []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"fields"`
		}
		if json.Unmarshal(sc.Bytes(), &raw) != nil {
			continue
		}
		l := LogLine{Time: time.Now(), Level: raw.Level, Message: raw.Message}
		if raw.Payload != "" || raw.Type != "" {
			l.Level, l.Message = raw.Type, raw.Payload
		}
		for _, f := range raw.Fields {
			l.Message += " " + f.Key + "=" + f.Value
		}
		// The router's clock is often in UTC without a zone; for a live stream the moment
		// the line arrives is exact enough and always in the viewer's time.
		l.Time = time.Now()
		if l.Level == "warn" {
			l.Level = "warning"
		}
		fn(l)
	}
	if ctx.Err() != nil {
		return nil
	}
	return sc.Err()
}
