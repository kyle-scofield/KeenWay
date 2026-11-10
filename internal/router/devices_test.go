package router

import "testing"

func TestParseHotspot(t *testing.T) {
	out := `
                  mac: 94:c6:91:99:99:6c
                   ip: 192.168.1.93
             hostname: laptop
                 name: laptop
            interface: 
                     name: Home
           registered: yes
               active: yes
                  mac: B6:62:EE:55:EA:4E
                   ip: 0.0.0.0
             hostname: 
                 name: POCO-X4-Pro-5G
            interface: 
                     name: Home
           registered: no
               active: no
`
	d := ParseHotspot(out)
	if len(d) != 2 {
		t.Fatalf("got %d devices", len(d))
	}
	if d[0].IP != "192.168.1.93" || d[0].Name != "laptop" || !d[0].Active || !d[0].Registered {
		t.Errorf("first: %+v", d[0])
	}
	if d[1].MAC != "b6:62:ee:55:ea:4e" || d[1].IP != "" || d[1].Name != "POCO-X4-Pro-5G" || d[1].Label() != "POCO-X4-Pro-5G" {
		t.Errorf("second: %+v", d[1])
	}
}
