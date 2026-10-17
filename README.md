# sinty-hostname

A native replacement for `hostnamectl` and `systemd-hostnamed` on SintyOS. It
shows and sets the computer name (static, pretty and transient), the icon name,
chassis, deployment and location, and reports the machine details people look
for in `hostnamectl`.

## Why it exists

SintyOS does not boot with systemd. PID 1 is `sinit` and services are driven by
`atomctl`. Without `systemd-hostnamed` nothing answers on
`org.freedesktop.hostname1`, so the computer name in Settings and in the Files
sidebar cannot be read or changed, and `hostnamectl` refuses to run:

    System has not been booted with systemd as init system (PID 1). Can't operate.

`sinty-hostname` works with the kernel and plain files directly:

- the transient name is the kernel hostname, read with `gethostname` and set
  with `sethostname`;
- the static name lives in `/var/lib/singularity/hostname`, the same file the
  first-boot setup writes and `atom-hostname-setup` applies at boot;
- the pretty name, icon name, chassis, deployment and location live in
  `/var/lib/singularity/machine-info`, in the `machine-info` format, layered
  over a read-only `/etc/machine-info` if the image ships one;
- the default name comes from `DEFAULT_HOSTNAME` in `os-release`, then
  `/etc/hostname`, then `sinty`;
- the chassis is detected from the DMI chassis type, and reported as `vm` on
  a hypervisor.

## Usage

    sinty-hostname [status]                 show the hostname and machine details
    sinty-hostname hostname [NAME]          show or set the hostname (root)
        --static | --pretty | --transient   limit to one of the three names
    sinty-hostname icon-name [NAME]         show or set the icon name (root)
    sinty-hostname chassis [TYPE]           show or set the chassis type (root)
    sinty-hostname deployment [NAME]        show or set the deployment (root)
    sinty-hostname location [TEXT]          show or set the location (root)
    sinty-hostname --help

`status` prints the same fields, in the same layout, as `hostnamectl`:

    $ sinty-hostname
       Static hostname: living-room-pc
       Pretty hostname: Living Room PC
             Icon name: computer-laptop
               Chassis: laptop
            Machine ID: 2b4f0d7e9c6a4f1e8d3c5b7a9e1f0c2d
      Operating System: Sinty OS Event Horizon 26
                Kernel: Linux 7.2.11
          Architecture: x86-64
       Hardware Vendor: LENOVO
        Hardware Model: Yoga Slim 6
      Firmware Version: KYCN31WW

Setting a name that is not a valid hostname, such as "Living Room PC", keeps it
as the pretty name and derives the static hostname from it (`living-room-pc`),
as `hostnamectl` does. An empty name resets the static hostname to the default.

Hostnames follow RFC 1123: labels of letters, digits and hyphens, no hyphen at
either end, at most 64 characters in all. `localhost` is reserved.

## Daemon

`sinty-hostnamed` serves the same logic over D-Bus as
`org.freedesktop.hostname1`, so Settings, the Files sidebar and any client of
the systemd API work without `systemd-hostnamed`. It implements `SetHostname`,
`SetStaticHostname`, `SetPrettyHostname`, `SetIconName`, `SetChassis`,
`SetDeployment`, `SetLocation` and `Describe`, and the `Hostname`,
`StaticHostname`, `PrettyHostname`, `DefaultHostname`, `HostnameSource`,
`IconName`, `Chassis`, `Deployment`, `Location`, kernel, operating system and
hardware properties.

Setting the static hostname also updates the kernel hostname, so the new name
is in effect without a reboot.

Callers are authorized through polkit with the systemd action ids
(`org.freedesktop.hostname1.set-hostname`,
`org.freedesktop.hostname1.set-static-hostname` and
`org.freedesktop.hostname1.set-machine-info`) when polkit is running, and by
membership of `wheel` (or `sudo`, `admin`) when it is not.

Packaging files (bus policy, activation file, unit) are in `packaging/`.

## Build

    make build            # ./sinty-hostname and ./sinty-hostnamed
    make static           # stripped static binaries for the image
    make test
    go build ./...        # plain build

Requires Go >= 1.25 and Linux. The non-stdlib dependencies are
`golang.org/x/sys/unix`, for `sethostname` and `uname`, and
`github.com/godbus/dbus/v5` for the daemon.
