package hostnamed

import (
	"encoding/json"
	"errors"
	"os/user"
	"strconv"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/singularityos-lab/sinty-hostname/internal/hostname"
)

const (
	BusName    = "org.freedesktop.hostname1"
	ObjectPath = dbus.ObjectPath("/org/freedesktop/hostname1")
	Iface      = "org.freedesktop.hostname1"
	propsIface = "org.freedesktop.DBus.Properties"

	actionHostname       = "org.freedesktop.hostname1.set-hostname"
	actionStaticHostname = "org.freedesktop.hostname1.set-static-hostname"
	actionMachineInfo    = "org.freedesktop.hostname1.set-machine-info"
	polkitName           = "org.freedesktop.PolicyKit1"
)

type Backend interface {
	Transient() string
	Static() string
	Default() string
	Source() string
	SetTransient(name string) error
	SetStatic(name string) error
	MachineInfo() map[string]string
	SetMachineInfo(key, value string) error
	Chassis() string
	IconName() string
	OSRelease() map[string]string
	HardwareVendor() string
	HardwareModel() string
	FirmwareVersion() string
}

type Service struct {
	conn        *dbus.Conn
	backend     Backend
	mu          sync.Mutex
	adminGroups []string
	isAdminHook func(uint32) bool
}

func New(conn *dbus.Conn, backend Backend) *Service {
	return &Service{conn: conn, backend: backend, adminGroups: []string{"wheel", "sudo", "admin"}}
}

func method(name string, args ...introspect.Arg) introspect.Method {
	return introspect.Method{Name: name, Args: args}
}

func in(name, typ string) introspect.Arg {
	return introspect.Arg{Name: name, Type: typ, Direction: "in"}
}
func out(name, typ string) introspect.Arg {
	return introspect.Arg{Name: name, Type: typ, Direction: "out"}
}

var propertyNames = []string{
	"Hostname", "StaticHostname", "PrettyHostname", "DefaultHostname", "HostnameSource",
	"IconName", "Chassis", "Deployment", "Location",
	"KernelName", "KernelRelease", "KernelVersion",
	"OperatingSystemPrettyName", "OperatingSystemCPEName", "OperatingSystemHomeURL",
	"HardwareVendor", "HardwareModel", "FirmwareVersion",
}

func introspection() introspect.Interface {
	props := make([]introspect.Property, 0, len(propertyNames))
	for _, n := range propertyNames {
		props = append(props, introspect.Property{Name: n, Type: "s", Access: "read"})
	}
	return introspect.Interface{
		Name: Iface,
		Methods: []introspect.Method{
			method("SetHostname", in("hostname", "s"), in("interactive", "b")),
			method("SetStaticHostname", in("hostname", "s"), in("interactive", "b")),
			method("SetPrettyHostname", in("hostname", "s"), in("interactive", "b")),
			method("SetIconName", in("icon", "s"), in("interactive", "b")),
			method("SetChassis", in("chassis", "s"), in("interactive", "b")),
			method("SetDeployment", in("deployment", "s"), in("interactive", "b")),
			method("SetLocation", in("location", "s"), in("interactive", "b")),
			method("Describe", out("json", "s")),
		},
		Properties: props,
	}
}

var propsIntrospection = introspect.Interface{
	Name: propsIface,
	Methods: []introspect.Method{
		method("Get", in("interface_name", "s"), in("property_name", "s"), out("value", "v")),
		method("GetAll", in("interface_name", "s"), out("properties", "a{sv}")),
		method("Set", in("interface_name", "s"), in("property_name", "s"), in("value", "v")),
	},
	Signals: []introspect.Signal{
		{Name: "PropertiesChanged", Args: []introspect.Arg{{Name: "interface_name", Type: "s"}, {Name: "changed_properties", Type: "a{sv}"}, {Name: "invalidated_properties", Type: "as"}}},
	},
}

func (s *Service) Export() error {
	if err := s.conn.ExportMethodTable(map[string]interface{}{
		"SetHostname":       s.SetHostname,
		"SetStaticHostname": s.SetStaticHostname,
		"SetPrettyHostname": s.SetPrettyHostname,
		"SetIconName":       s.SetIconName,
		"SetChassis":        s.SetChassis,
		"SetDeployment":     s.SetDeployment,
		"SetLocation":       s.SetLocation,
		"Describe":          s.Describe,
	}, ObjectPath, Iface); err != nil {
		return err
	}
	if err := s.conn.ExportMethodTable(map[string]interface{}{
		"Get":    s.get,
		"GetAll": s.getAll,
		"Set":    s.set,
	}, ObjectPath, propsIface); err != nil {
		return err
	}
	node := &introspect.Node{Name: string(ObjectPath), Interfaces: []introspect.Interface{introspect.IntrospectData, propsIntrospection, introspection()}}
	return s.conn.Export(introspect.NewIntrospectable(node), ObjectPath, "org.freedesktop.DBus.Introspectable")
}

func (s *Service) values() map[string]string {
	info := s.backend.MachineInfo()
	osr := s.backend.OSRelease()
	k := hostname.KernelInfo()
	return map[string]string{
		"Hostname":                  s.backend.Transient(),
		"StaticHostname":            s.backend.Static(),
		"PrettyHostname":            info["PRETTY_HOSTNAME"],
		"DefaultHostname":           s.backend.Default(),
		"HostnameSource":            s.backend.Source(),
		"IconName":                  s.backend.IconName(),
		"Chassis":                   s.backend.Chassis(),
		"Deployment":                info["DEPLOYMENT"],
		"Location":                  info["LOCATION"],
		"KernelName":                k.Name,
		"KernelRelease":             k.Release,
		"KernelVersion":             k.Version,
		"OperatingSystemPrettyName": osr["PRETTY_NAME"],
		"OperatingSystemCPEName":    osr["CPE_NAME"],
		"OperatingSystemHomeURL":    osr["HOME_URL"],
		"HardwareVendor":            s.backend.HardwareVendor(),
		"HardwareModel":             s.backend.HardwareModel(),
		"FirmwareVersion":           s.backend.FirmwareVersion(),
	}
}

func (s *Service) props() map[string]dbus.Variant {
	out := map[string]dbus.Variant{}
	for k, v := range s.values() {
		out[k] = dbus.MakeVariant(v)
	}
	return out
}

func (s *Service) get(iface, name string) (dbus.Variant, *dbus.Error) {
	all, err := s.getAll(iface)
	if err != nil {
		return dbus.Variant{}, err
	}
	v, ok := all[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []interface{}{name})
	}
	return v, nil
}

func (s *Service) getAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	if iface != Iface {
		return nil, dbus.NewError("org.freedesktop.DBus.Error.UnknownInterface", []interface{}{iface})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.props(), nil
}

func (s *Service) set(iface, name string, v dbus.Variant) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly", []interface{}{name})
}

func (s *Service) changed(names ...string) {
	all := s.props()
	out := map[string]dbus.Variant{}
	for _, n := range names {
		out[n] = all[n]
	}
	s.conn.Emit(ObjectPath, propsIface+".PropertiesChanged", Iface, out, []string{})
}

func failed(err error) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.Failed", []interface{}{err.Error()})
}

func invalid(err error) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []interface{}{err.Error()})
}

var errDenied = errors.New("not authorized")

func (s *Service) authorize(sender dbus.Sender, action string, interactive bool) *dbus.Error {
	var uid uint32
	if err := s.conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixUser", 0, string(sender)).Store(&uid); err != nil {
		return failed(err)
	}
	if uid == 0 {
		return nil
	}
	if s.hasPolkit() {
		subject := struct {
			Kind    string
			Details map[string]dbus.Variant
		}{"system-bus-name", map[string]dbus.Variant{"name": dbus.MakeVariant(string(sender))}}
		var res struct {
			Authorized bool
			Challenge  bool
			Details    map[string]string
		}
		flags := uint32(0)
		if interactive {
			flags = 1
		}
		obj := s.conn.Object(polkitName, "/org/freedesktop/PolicyKit1/Authority")
		if err := obj.Call("org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0, subject, action, map[string]string{}, flags, "").Store(&res); err == nil {
			if res.Authorized {
				return nil
			}
			return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{errDenied.Error()})
		}
	}
	if s.isAdmin(uid) {
		return nil
	}
	return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []interface{}{errDenied.Error()})
}

func (s *Service) hasPolkit() bool {
	var owned bool
	if err := s.conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, polkitName).Store(&owned); err == nil && owned {
		return true
	}
	var names []string
	if err := s.conn.BusObject().Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&names); err != nil {
		return false
	}
	for _, n := range names {
		if n == polkitName {
			return true
		}
	}
	return false
}

func (s *Service) isAdmin(uid uint32) bool {
	if s.isAdminHook != nil {
		return s.isAdminHook(uid)
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return false
	}
	ids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, id := range ids {
		g, err := user.LookupGroupId(id)
		if err != nil {
			continue
		}
		for _, want := range s.adminGroups {
			if g.Name == want {
				return true
			}
		}
	}
	return false
}

func (s *Service) SetHostname(name string, interactive bool, sender dbus.Sender) *dbus.Error {
	if err := s.authorize(sender, actionHostname, interactive); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		name = s.backend.Static()
		if name == "" {
			name = s.backend.Default()
		}
	}
	if err := hostname.ValidHostname(name); err != nil {
		return invalid(err)
	}
	if name == s.backend.Transient() {
		return nil
	}
	if err := s.backend.SetTransient(name); err != nil {
		return failed(err)
	}
	s.changed("Hostname")
	return nil
}

func (s *Service) SetStaticHostname(name string, interactive bool, sender dbus.Sender) *dbus.Error {
	if err := s.authorize(sender, actionStaticHostname, interactive); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		if err := hostname.ValidHostname(name); err != nil {
			return invalid(err)
		}
	}
	if name == s.backend.Static() {
		return nil
	}
	if err := s.backend.SetStatic(name); err != nil {
		return failed(err)
	}
	transient := name
	if transient == "" {
		transient = s.backend.Default()
	}
	if err := s.backend.SetTransient(transient); err != nil {
		return failed(err)
	}
	s.changed("StaticHostname", "Hostname", "HostnameSource")
	return nil
}

func (s *Service) setInfo(key, prop, value string, interactive bool, sender dbus.Sender) *dbus.Error {
	if err := s.authorize(sender, actionMachineInfo, interactive); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend.MachineInfo()[key] == value {
		return nil
	}
	if err := s.backend.SetMachineInfo(key, value); err != nil {
		return invalid(err)
	}
	if key == "CHASSIS" {
		s.changed(prop, "IconName")
	} else {
		s.changed(prop)
	}
	return nil
}

func (s *Service) SetPrettyHostname(name string, interactive bool, sender dbus.Sender) *dbus.Error {
	return s.setInfo("PRETTY_HOSTNAME", "PrettyHostname", name, interactive, sender)
}

func (s *Service) SetIconName(icon string, interactive bool, sender dbus.Sender) *dbus.Error {
	return s.setInfo("ICON_NAME", "IconName", icon, interactive, sender)
}

func (s *Service) SetChassis(chassis string, interactive bool, sender dbus.Sender) *dbus.Error {
	return s.setInfo("CHASSIS", "Chassis", chassis, interactive, sender)
}

func (s *Service) SetDeployment(deployment string, interactive bool, sender dbus.Sender) *dbus.Error {
	return s.setInfo("DEPLOYMENT", "Deployment", deployment, interactive, sender)
}

func (s *Service) SetLocation(location string, interactive bool, sender dbus.Sender) *dbus.Error {
	return s.setInfo("LOCATION", "Location", location, interactive, sender)
}

func (s *Service) Describe() (string, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.values())
	if err != nil {
		return "", failed(err)
	}
	return string(data), nil
}
