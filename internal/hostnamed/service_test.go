package hostnamed

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/singularityos-lab/sinty-hostname/internal/hostname"
)

func privateBus(t *testing.T) string {
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skip("dbus-daemon not available")
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

func TestService(t *testing.T) {
	addr := privateBus(t)
	server, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "etc"), 0o755)
	os.WriteFile(filepath.Join(dir, "etc/hostname"), []byte("sinty\n"), 0o644)
	sys := hostname.System{Root: dir}
	sys.SetTransient("sinty")
	svc := New(server, sys)
	var allow atomic.Bool
	svc.isAdminHook = func(uint32) bool { return allow.Load() }
	if err := svc.Export(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.RequestName(BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	client, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	obj := client.Object(BusName, ObjectPath)

	src, err := obj.GetProperty(Iface + ".HostnameSource")
	if err != nil || src.Value().(string) != "default" {
		t.Fatalf("HostnameSource = %v %v", src, err)
	}
	if err := obj.Call(Iface+".SetStaticHostname", 0, "studio", false).Err; err == nil {
		t.Fatal("a non-admin caller without polkit must be refused")
	}
	allow.Store(true)
	if err := obj.Call(Iface+".SetStaticHostname", 0, "bad name", false).Err; err == nil {
		t.Fatal("invalid hostname accepted")
	}
	if err := obj.Call(Iface+".SetPrettyHostname", 0, "Living Room PC", true).Err; err != nil {
		t.Fatal(err)
	}
	if err := obj.Call(Iface+".SetStaticHostname", 0, "living-room-pc", true).Err; err != nil {
		t.Fatal(err)
	}
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, Iface).Store(&props); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"StaticHostname": "living-room-pc", "Hostname": "living-room-pc", "PrettyHostname": "Living Room PC", "HostnameSource": "static"}
	for k, v := range want {
		if props[k].Value().(string) != v {
			t.Errorf("%s = %v, want %q", k, props[k].Value(), v)
		}
	}
	if err := obj.Call(Iface+".SetChassis", 0, "laptop", false).Err; err != nil {
		t.Fatal(err)
	}
	icon, _ := obj.GetProperty(Iface + ".IconName")
	if icon.Value().(string) != "computer-laptop" {
		t.Fatalf("IconName = %v", icon.Value())
	}
	if err := obj.Call(Iface+".SetStaticHostname", 0, "", false).Err; err != nil {
		t.Fatal(err)
	}
	var js string
	if err := obj.Call(Iface+".Describe", 0).Store(&js); err != nil {
		t.Fatal(err)
	}
	var desc map[string]string
	if err := json.Unmarshal([]byte(js), &desc); err != nil {
		t.Fatal(err)
	}
	if desc["StaticHostname"] != "" || desc["Hostname"] != "sinty" || desc["HostnameSource"] != "default" {
		t.Fatalf("after reset: %v", desc)
	}
}
