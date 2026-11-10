// routercheck exercises the read-only router operations against a real router.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"keenway/internal/router"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t := router.Target{Host: os.Getenv("ROUTER_HOST"), User: "admin", Password: os.Getenv("ROUTER_PASS")}
	c, err := router.Connect(ctx, t)
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	defer c.Close()
	fmt.Printf("connected: port=%d cli=%v hostkey=%s\n", c.Port, c.CLI, c.NewHostKey)
	devs, err := c.Devices(ctx)
	fmt.Printf("devices: %d err=%v\n", len(devs), err)
	for _, d := range devs {
		if d.IP != "" {
			fmt.Printf("  %-22s %-15s active=%v\n", d.Label(), d.IP, d.Active)
		}
	}
	on, err := c.TemplateUpdaterEnabled(ctx)
	fmt.Println("template updater enabled:", on, err)
	b, err := c.Backups(ctx)
	fmt.Println("keenway backups:", len(b), err)
	cfg, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("read config:", err)
		os.Exit(1)
	}
	err = c.Apply(ctx, cfg, router.ApplyOptions{DryRun: true, Progress: func(s router.Step) { fmt.Printf("  step: %s ok=%v %s\n", s.Name, s.OK, s.Info) }})
	fmt.Println("dry-run apply:", err)
	live, err := c.CurrentConfig(ctx)
	fmt.Println("live config bytes:", len(live), err)
}
