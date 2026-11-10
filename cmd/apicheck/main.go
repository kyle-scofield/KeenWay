// apicheck reads status from a real mihomo panel (read-only).
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"keenway/internal/mihomo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	c := mihomo.New(os.Getenv("ROUTER_HOST"), os.Getenv("MIHOMO_SECRET"))
	v, err := c.Version(ctx)
	fmt.Println("version:", v, err)
	gs, err := c.Groups(ctx)
	fmt.Println("groups:", len(gs), err)
	for _, g := range gs[:min(5, len(gs))] {
		fmt.Printf("  %-22s now=%-28s options=%d hidden=%v\n", g.Name, g.Now, len(g.Options), g.Hidden)
	}
	ps, err := c.Providers(ctx)
	fmt.Println("providers:", len(ps), err)
	for _, p := range ps {
		fmt.Printf("  %s nodes=%d alive=%d updated=%s\n", p.Name, len(p.Nodes), p.Alive, p.UpdatedAt.Format(time.DateTime))
		if p.Info != nil {
			days := "без срока"
			if p.Info.Expire > 0 {
				days = fmt.Sprintf("%d дн.", int(time.Until(time.Unix(p.Info.Expire, 0)).Hours()/24))
			}
			fmt.Printf("    used=%.1f GB total=%.1f GB left=%s\n", float64(p.Info.Upload+p.Info.Download)/1e9, float64(p.Info.Total)/1e9, days)
		}
	}
	s, err := c.Traffic(ctx)
	if err == nil {
		fmt.Printf("traffic total: up=%.2f GB down=%.2f GB, open connections=%d\n", float64(s.UploadTotal)/1e9, float64(s.DownloadTotal)/1e9, len(s.Connections))
		for _, u := range s.ByService()[:min(4, len(s.ByService()))] {
			fmt.Printf("  service %-20s %d conns, %.1f MB\n", u.Key, u.Connections, float64(u.Upload+u.Download)/1e6)
		}
	} else {
		fmt.Println("traffic:", err)
	}
}
