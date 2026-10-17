package hostname

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const (
	StateDir        = "/var/lib/singularity"
	FallbackDefault = "sinty"
	maxHostname     = 64
	maxPretty       = 255
)

var Chassis = []string{"desktop", "laptop", "convertible", "server", "tablet", "handset", "watch", "embedded", "vm", "container"}

var machineInfoKeys = []string{"PRETTY_HOSTNAME", "ICON_NAME", "CHASSIS", "DEPLOYMENT", "LOCATION"}

type System struct {
	Root string
}

func (s System) path(p string) string {
	if s.Root == "" {
		return p
	}
	return filepath.Join(s.Root, p)
}

func (s System) StaticFile() string      { return s.path(filepath.Join(StateDir, "hostname")) }
func (s System) MachineInfoFile() string { return s.path(filepath.Join(StateDir, "machine-info")) }

func firstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line)
}

func (s System) Static() string { return firstLine(s.StaticFile()) }

func (s System) Default() string {
	if h := s.OSRelease()["DEFAULT_HOSTNAME"]; ValidHostname(h) == nil {
		return h
	}
	if h := firstLine(s.path("/etc/hostname")); ValidHostname(h) == nil {
		return h
	}
	return FallbackDefault
}

func (s System) Transient() string {
	if s.Root != "" {
		return firstLine(s.path("/proc/sys/kernel/hostname"))
	}
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

func (s System) Source() string {
	if s.Static() != "" {
		return "static"
	}
	return "default"
}

func (s System) SetTransient(name string) error {
	if err := ValidHostname(name); err != nil {
		return err
	}
	if s.Root != "" {
		p := s.path("/proc/sys/kernel/hostname")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(name+"\n"), 0o644)
	}
	return unix.Sethostname([]byte(name))
}

func (s System) SetStatic(name string) error {
	if name == "" {
		err := os.Remove(s.StaticFile())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := ValidHostname(name); err != nil {
		return err
	}
	return writeFileAtomic(s.StaticFile(), []byte(name+"\n"), 0o644)
}

func ValidHostname(name string) error {
	if name == "" {
		return errors.New("hostname is empty")
	}
	if len(name) > maxHostname {
		return fmt.Errorf("hostname is longer than %d characters", maxHostname)
	}
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return errors.New("localhost is reserved")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			return errors.New("hostname has an empty label")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("hostname labels cannot start or end with a hyphen")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("hostname contains %q", c)
			}
		}
	}
	return nil
}

func ValidPretty(name string) error {
	if len(name) > maxPretty {
		return fmt.Errorf("pretty hostname is longer than %d bytes", maxPretty)
	}
	if !utf8.ValidString(name) {
		return errors.New("pretty hostname is not valid UTF-8")
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return errors.New("pretty hostname contains a control character")
		}
	}
	return nil
}

func ValidChassis(c string) error {
	if c == "" {
		return nil
	}
	for _, k := range Chassis {
		if c == k {
			return nil
		}
	}
	return fmt.Errorf("unknown chassis %q", c)
}

func validField(key, value string) error {
	switch key {
	case "PRETTY_HOSTNAME", "LOCATION":
		return ValidPretty(value)
	case "CHASSIS":
		return ValidChassis(value)
	case "ICON_NAME", "DEPLOYMENT":
		for _, c := range value {
			if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' || c == '.') {
				return fmt.Errorf("%s contains %q", strings.ToLower(key), c)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown field %s", key)
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		inner := v[1 : len(v)-1]
		if v[0] == '\'' {
			return inner
		}
		var b strings.Builder
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				i++
			}
			b.WriteByte(inner[i])
		}
		return b.String()
	}
	return v
}

func quote(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range v {
		if c == '"' || c == '\\' || c == '$' || c == '`' {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	b.WriteByte('"')
	return b.String()
}

func parseEnvFile(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = unquote(strings.TrimSpace(v))
	}
	return out
}

func (s System) MachineInfo() map[string]string {
	info := parseEnvFile(s.path("/etc/machine-info"))
	for k, v := range parseEnvFile(s.MachineInfoFile()) {
		info[k] = v
	}
	return info
}

func (s System) SetMachineInfo(key, value string) error {
	if err := validField(key, value); err != nil {
		return err
	}
	info := parseEnvFile(s.MachineInfoFile())
	if value == "" {
		delete(info, key)
	} else {
		info[key] = value
	}
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + quote(info[k]) + "\n")
	}
	if b.Len() == 0 {
		err := os.Remove(s.MachineInfoFile())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeFileAtomic(s.MachineInfoFile(), []byte(b.String()), 0o644)
}

func (s System) OSRelease() map[string]string {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if info := parseEnvFile(s.path(p)); len(info) > 0 {
			return info
		}
	}
	return map[string]string{}
}

func (s System) dmi(name string) string {
	return firstLine(s.path(filepath.Join("/sys/class/dmi/id", name)))
}

func (s System) HardwareVendor() string  { return s.dmi("sys_vendor") }
func (s System) HardwareModel() string   { return s.dmi("product_name") }
func (s System) FirmwareVersion() string { return s.dmi("bios_version") }

func (s System) MachineID() string { return firstLine(s.path("/etc/machine-id")) }

func (s System) virtualized() bool {
	switch strings.ToLower(s.HardwareVendor()) {
	case "qemu", "innotek gmbh", "vmware, inc.", "microsoft corporation", "xen", "parallels software international inc.":
		return true
	}
	data, err := os.ReadFile(s.path("/proc/cpuinfo"))
	return err == nil && strings.Contains(string(data), " hypervisor")
}

func (s System) DetectChassis() string {
	if s.virtualized() {
		return "vm"
	}
	n, err := strconv.Atoi(s.dmi("chassis_type"))
	if err != nil {
		return ""
	}
	switch n {
	case 3, 4, 6, 7, 13, 15, 16, 24:
		return "desktop"
	case 8, 9, 10, 14:
		return "laptop"
	case 11:
		return "handset"
	case 17, 23, 25, 28, 29:
		return "server"
	case 30:
		return "tablet"
	case 31, 32:
		return "convertible"
	case 33, 34, 35, 36:
		return "embedded"
	}
	return ""
}

func (s System) Chassis() string {
	if c := s.MachineInfo()["CHASSIS"]; c != "" {
		return c
	}
	return s.DetectChassis()
}

func (s System) IconName() string {
	if i := s.MachineInfo()["ICON_NAME"]; i != "" {
		return i
	}
	switch c := s.Chassis(); c {
	case "":
		return "computer"
	case "vm":
		return "computer-vm"
	default:
		return "computer-" + c
	}
}

type Kernel struct {
	Name, Release, Version string
}

func KernelInfo() Kernel {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return Kernel{}
	}
	return Kernel{unix.ByteSliceToString(u.Sysname[:]), unix.ByteSliceToString(u.Release[:]), unix.ByteSliceToString(u.Version[:])}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sinty-hostname-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
