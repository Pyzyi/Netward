// Package privdrop prepares the process for running without privileges.
//
// The container starts as root with exactly CHOWN, SETUID, SETGID and
// NET_RAW. Prepare hands the data directory to the runtime user, opens every
// raw socket Netzwart will need and then switches to PUID:PGID. Because the
// switch happens without PR_SET_KEEPCAPS, the kernel clears all capability
// sets: the long-running process has no capabilities at all. Raw sockets
// keep working because CAP_NET_RAW is only checked when a socket is created.
package privdrop

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/Pyzyi/Netward/internal/config"
	"github.com/Pyzyi/Netward/internal/l2"
	"github.com/Pyzyi/Netward/internal/netinfo"
)

// Result describes what Prepare achieved.
type Result struct {
	Links   []*l2.Link
	Pinger  *l2.Pinger
	Dropped bool // switched from root to PUID:PGID
	UID     int
	GID     int
	CapEff  string // effective capabilities after the switch (hex)
	// Decoy listeners from NETZWART_TRIPWIRE_PORTS (docs/10, F3). Ports
	// below 1024 can only be opened here, before the switch, and only with
	// cap_add NET_BIND_SERVICE.
	Tripwire map[int]net.Listener
}

// hardenProcess applies settings that need no privileges.
func hardenProcess() {
	unix.Umask(0o077)
	// Equivalent to Docker's no-new-privileges, but independent of the compose file.
	_ = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
}

// Prepare opens raw sockets and drops privileges (see package doc).
func Prepare(env config.Env, log *slog.Logger) (Result, error) {
	hardenProcess()
	res := Result{UID: os.Getuid(), GID: os.Getgid()}
	root := os.Geteuid() == 0

	if root {
		if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
			log.Warn("cannot create data directory", "dir", env.DataDir, "err", err)
		}
		// Non-recursive on purpose: root here has no DAC_OVERRIDE and must not
		// traverse directories it does not own.
		if err := os.Lchown(env.DataDir, env.PUID, env.PGID); err != nil {
			log.Warn("cannot hand data directory to runtime user", "dir", env.DataDir, "err", err)
		}
	}

	ifs, err := netinfo.Interfaces(nil)
	if err != nil {
		log.Warn("cannot list interfaces", "err", err)
	}
	if len(env.Interfaces) > 0 {
		extra, _ := netinfo.Interfaces(env.Interfaces)
		ifs = append(ifs, extra...)
	}
	seen := map[int]bool{}
	for _, i := range ifs {
		if seen[i.Index] {
			continue
		}
		seen[i.Index] = true
		link, err := l2.Open(i)
		if errors.Is(err, l2.ErrNoRawSocket) {
			log.Warn("raw sockets unavailable, using the slower unprivileged scan mode",
				"hint", "start the container as root with cap_add NET_RAW (see README)")
			break
		}
		if err != nil {
			log.Warn("cannot open interface", "iface", i.Name, "err", err)
			continue
		}
		res.Links = append(res.Links, link)
	}
	res.Pinger = l2.OpenPinger()
	res.Tripwire = openTripwire(env.TripwirePorts, log)

	if root {
		if err := Drop(env.PUID, env.PGID); err != nil {
			return res, err
		}
		res.Dropped = true
		res.UID, res.GID = env.PUID, env.PGID
	}
	res.CapEff = CapEff()
	return res, nil
}

func openTripwire(ports []int, log *slog.Logger) map[int]net.Listener {
	out := map[int]net.Listener{}
	for _, p := range ports {
		l, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err != nil {
			hint := ""
			if p < 1024 && errors.Is(err, syscall.EACCES) {
				hint = "add cap_add NET_BIND_SERVICE to the container (see README)"
			}
			log.Warn("tripwire port not opened", "port", p, "err", err, "hint", hint)
			continue
		}
		out[p] = l
	}
	return out
}

// Drop switches all threads to uid:gid with no supplementary groups. Without
// PR_SET_KEEPCAPS the kernel clears every capability during setresuid.
func Drop(uid, gid int) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		return errors.New("privilege drop failed: still running as root")
	}
	if err := syscall.Setuid(0); err == nil {
		return errors.New("privilege drop failed: could regain root")
	}
	return nil
}

// DropForCommand is used by CLI subcommands (docker exec runs as root): it
// switches to the runtime user before any file in the data directory is
// touched, so no root-owned files are ever created there.
func DropForCommand(env config.Env) error {
	hardenProcess()
	return Drop(env.PUID, env.PGID)
}

// CapEff returns the effective capability mask of this process.
func CapEff() string {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "CapEff:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// CheckDataDir verifies (as the runtime user) that dir is usable and
// tightens its permissions.
func CheckDataDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("data directory %s is not accessible: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	_ = os.Chmod(dir, 0o700)
	probe := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		st, _ := fi.Sys().(*syscall.Stat_t)
		owner := -1
		if st != nil {
			owner = int(st.Uid)
		}
		return fmt.Errorf("data directory %s is not writable for uid %d (owned by uid %d); "+
			"fix with: chown -R %d:%d <host path of the volume>", dir, os.Getuid(), owner, os.Getuid(), os.Getgid())
	}
	return os.Remove(probe)
}
