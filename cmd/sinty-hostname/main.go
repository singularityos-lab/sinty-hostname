package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/singularityos-lab/sinty-hostname/internal/hostname"
)

var sys = hostname.System{}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return cmdStatus()
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return 0
	case "status":
		return cmdStatus()
	case "hostname", "set-hostname":
		return cmdHostname(args[1:])
	case "icon-name":
		return cmdInfo("icon-name", "ICON_NAME", sys.IconName, args[1:])
	case "chassis":
		return cmdInfo("chassis", "CHASSIS", sys.Chassis, args[1:])
	case "deployment":
		return cmdInfo("deployment", "DEPLOYMENT", func() string { return sys.MachineInfo()["DEPLOYMENT"] }, args[1:])
	case "location":
		return cmdInfo("location", "LOCATION", func() string { return sys.MachineInfo()["LOCATION"] }, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "sinty-hostname: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func line(label, value string) {
	if value != "" {
		fmt.Printf("%17s: %s\n", label, value)
	}
}

func arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86-64"
	case "arm64":
		return "arm64"
	}
	return runtime.GOARCH
}

func cmdStatus() int {
	static := sys.Static()
	transient := sys.Transient()
	info := sys.MachineInfo()
	k := hostname.KernelInfo()
	if static != "" {
		line("Static hostname", static)
	} else {
		line("Static hostname", "(unset)")
	}
	if transient != "" && transient != static {
		line("Transient hostname", transient)
	}
	line("Pretty hostname", info["PRETTY_HOSTNAME"])
	line("Icon name", sys.IconName())
	line("Chassis", sys.Chassis())
	line("Deployment", info["DEPLOYMENT"])
	line("Location", info["LOCATION"])
	line("Machine ID", sys.MachineID())
	line("Operating System", sys.OSRelease()["PRETTY_NAME"])
	if k.Name != "" {
		line("Kernel", k.Name+" "+k.Release)
	}
	line("Architecture", arch())
	line("Hardware Vendor", sys.HardwareVendor())
	line("Hardware Model", sys.HardwareModel())
	line("Firmware Version", sys.FirmwareVersion())
	return 0
}

func Slug(pretty string) string {
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(pretty) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			dash = false
		case c == '-' || c == ' ' || c == '_' || c == '.':
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

func cmdHostname(args []string) int {
	static, pretty, transient := false, false, false
	var rest []string
	for _, a := range args {
		switch a {
		case "--static":
			static = true
		case "--pretty":
			pretty = true
		case "--transient":
			transient = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		switch {
		case pretty:
			fmt.Println(sys.MachineInfo()["PRETTY_HOSTNAME"])
		case static:
			fmt.Println(sys.Static())
		default:
			fmt.Println(sys.Transient())
		}
		return 0
	}
	if len(rest) != 1 {
		return fail("hostname: expected exactly one NAME argument")
	}
	if !isRoot() {
		return fail("hostname: must be run as root")
	}
	name := rest[0]
	if !static && !pretty && !transient {
		static, pretty, transient = true, true, true
	}
	clean := name
	if hostname.ValidHostname(name) != nil {
		clean = Slug(name)
	}
	if (static || transient) && name != "" {
		if err := hostname.ValidHostname(clean); err != nil {
			return fail("hostname: %v", err)
		}
	}
	if pretty {
		value := name
		if value == clean {
			value = ""
		}
		if err := sys.SetMachineInfo("PRETTY_HOSTNAME", value); err != nil {
			return fail("hostname: %v", err)
		}
	}
	if static {
		if err := sys.SetStatic(clean); err != nil {
			return fail("hostname: %v", err)
		}
	}
	if transient {
		target := clean
		if target == "" {
			target = sys.Default()
		}
		if err := sys.SetTransient(target); err != nil {
			return fail("hostname: %v", err)
		}
	}
	return 0
}

func cmdInfo(verb, key string, current func() string, args []string) int {
	if len(args) == 0 {
		fmt.Println(current())
		return 0
	}
	if len(args) != 1 {
		return fail("%s: expected exactly one argument", verb)
	}
	if !isRoot() {
		return fail("%s: must be run as root", verb)
	}
	if err := sys.SetMachineInfo(key, args[0]); err != nil {
		return fail("%s: %v", verb, err)
	}
	return 0
}

func isRoot() bool { return os.Geteuid() == 0 }

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "sinty-hostname: "+format+"\n", a...)
	return 1
}

func usage(w *os.File) {
	fmt.Fprint(w, `sinty-hostname - query and set the computer name on SintyOS

Usage:
  sinty-hostname [status]                       show the hostname and machine details
  sinty-hostname hostname [NAME]                show or set the hostname (root)
      --static | --pretty | --transient         limit to one of the three names
  sinty-hostname icon-name [NAME]               show or set the icon name (root)
  sinty-hostname chassis [TYPE]                 show or set the chassis type (root)
  sinty-hostname deployment [NAME]              show or set the deployment (root)
  sinty-hostname location [TEXT]                show or set the location (root)
  sinty-hostname --help

Setting a name that is not a valid hostname, such as "Living Room PC", keeps it
as the pretty name and derives the static hostname from it ("living-room-pc").
An empty NAME resets the static hostname to the default.
`)
}
