package hostname

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func root(t *testing.T, files map[string]string) System {
	dir := t.TempDir()
	for p, data := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return System{Root: dir}
}

func TestValidHostname(t *testing.T) {
	good := []string{"sinty", "living-room-pc", "a.b.c", "Box1", strings.Repeat("a", 63)}
	bad := []string{"", "-sinty", "sinty-", "a..b", "sinty pc", "café", "localhost", "x.localhost", strings.Repeat("a", 65), "a/b"}
	for _, h := range good {
		if err := ValidHostname(h); err != nil {
			t.Errorf("%q rejected: %v", h, err)
		}
	}
	for _, h := range bad {
		if ValidHostname(h) == nil {
			t.Errorf("%q accepted", h)
		}
	}
}

func TestStaticRoundTrip(t *testing.T) {
	s := root(t, map[string]string{"/etc/hostname": "sinty\n"})
	if s.Static() != "" || s.Source() != "default" || s.Default() != "sinty" {
		t.Fatalf("fresh: static %q source %q default %q", s.Static(), s.Source(), s.Default())
	}
	if err := s.SetStatic("studio"); err != nil {
		t.Fatal(err)
	}
	if s.Static() != "studio" || s.Source() != "static" {
		t.Fatalf("after set: %q %q", s.Static(), s.Source())
	}
	if err := s.SetStatic("bad name"); err == nil {
		t.Fatal("invalid static hostname accepted")
	}
	if err := s.SetStatic(""); err != nil {
		t.Fatal(err)
	}
	if s.Static() != "" {
		t.Fatalf("reset left %q", s.Static())
	}
	if err := s.SetTransient("studio"); err != nil || s.Transient() != "studio" {
		t.Fatalf("transient %q %v", s.Transient(), err)
	}
}

func TestDefaultFromOSRelease(t *testing.T) {
	s := root(t, map[string]string{"/usr/lib/os-release": "NAME=\"Sinty OS\"\nDEFAULT_HOSTNAME=event-horizon\n"})
	if s.Default() != "event-horizon" {
		t.Fatalf("default %q", s.Default())
	}
	s = root(t, nil)
	if s.Default() != FallbackDefault {
		t.Fatalf("fallback %q", s.Default())
	}
}

func TestMachineInfo(t *testing.T) {
	s := root(t, map[string]string{"/etc/machine-info": "LOCATION=Lab\n"})
	pretty := `Living Room "PC" $1 \ x`
	if err := s.SetMachineInfo("PRETTY_HOSTNAME", pretty); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMachineInfo("CHASSIS", "laptop"); err != nil {
		t.Fatal(err)
	}
	info := s.MachineInfo()
	if info["PRETTY_HOSTNAME"] != pretty || info["CHASSIS"] != "laptop" || info["LOCATION"] != "Lab" {
		t.Fatalf("info %q", info)
	}
	if s.IconName() != "computer-laptop" {
		t.Fatalf("icon %q", s.IconName())
	}
	if err := s.SetMachineInfo("CHASSIS", "spaceship"); err == nil {
		t.Fatal("unknown chassis accepted")
	}
	if err := s.SetMachineInfo("PRETTY_HOSTNAME", "a\nb"); err == nil {
		t.Fatal("control character accepted")
	}
	if err := s.SetMachineInfo("ICON_NAME", "x;rm"); err == nil {
		t.Fatal("bad icon name accepted")
	}
	if err := s.SetMachineInfo("PRETTY_HOSTNAME", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMachineInfo("CHASSIS", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.MachineInfoFile()); !os.IsNotExist(err) {
		t.Fatalf("empty machine-info kept: %v", err)
	}
}

func TestDetectChassis(t *testing.T) {
	cases := map[string]string{"10": "laptop", "3": "desktop", "31": "convertible", "17": "server", "99": ""}
	for typ, want := range cases {
		s := root(t, map[string]string{"/sys/class/dmi/id/chassis_type": typ + "\n", "/sys/class/dmi/id/sys_vendor": "LENOVO\n", "/proc/cpuinfo": "flags\t: fpu sse\n"})
		if got := s.DetectChassis(); got != want {
			t.Errorf("chassis_type %s: %q, want %q", typ, got, want)
		}
	}
	s := root(t, map[string]string{"/sys/class/dmi/id/chassis_type": "1\n", "/sys/class/dmi/id/sys_vendor": "QEMU\n"})
	if s.DetectChassis() != "vm" || s.IconName() != "computer-vm" {
		t.Fatalf("qemu: %q %q", s.DetectChassis(), s.IconName())
	}
}
