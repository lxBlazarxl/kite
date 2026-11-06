package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/retransmit/kite/internal/agent"
)

const serviceName = "KiteAgent"

func kiteDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "Kite")
}

func envPath() string { return filepath.Join(kiteDir(), "agent.env") }

// installedExePath is where install copies the agent: a folder only
// administrators can write, so the SYSTEM service never runs a binary an
// ordinary user could replace (for example in Downloads).
func installedExePath() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, "Kite", "kite-agent.exe")
}

// copyExe copies src to dst (replacing it), creating dst's folder.
func copyExe(src, dst string) error {
	if strings.EqualFold(filepath.Clean(src), filepath.Clean(dst)) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

var (
	sidAdministrators, _ = windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	sidSystem, _         = windows.CreateWellKnownSid(windows.WinLocalSystemSid)
)

// secureDir makes path a folder only SYSTEM and Administrators can use,
// inherited by everything inside it. ProgramData lets ordinary users create
// folders, so an existing one is only accepted when it is a real directory
// (not a junction or link) owned by Administrators or SYSTEM.
func secureDir(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if attrs, err := windows.GetFileAttributes(p); err == nil {
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("refusing to use %s: it is a reparse point (junction or link)", path)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		owner, _, err := sd.Owner()
		if err != nil {
			return err
		}
		if !owner.Equals(sidAdministrators) && !owner.Equals(sidSystem) {
			return fmt.Errorf("refusing to use %s: owned by %s, not Administrators; delete it and run install again", path, owner)
		}
	} else if err := os.Mkdir(path, 0o755); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
func logPath() string { return filepath.Join(kiteDir(), "agent.log") }

func requireAdmin() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("run this from an Administrator PowerShell")
	}
	return nil
}

// writePrivate writes the env file readable only by SYSTEM and
// Administrators, since it holds the agent token.
func writePrivate(path string, env map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := agent.WriteEnvFile(f, env); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func installService(args []string) error {
	if err := requireAdmin(); err != nil {
		return err
	}
	env, err := installArgsToEnv(args)
	if err != nil {
		return err
	}
	if err := secureDir(kiteDir()); err != nil {
		return err
	}
	if err := writePrivate(envPath(), env); err != nil {
		return fmt.Errorf("write %s: %w", envPath(), err)
	}
	src, err := os.Executable()
	if err != nil {
		return err
	}
	exe := installedExePath()
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		stopAndWait(s) // the running exe is locked; stop before replacing it
	}
	if err := copyExe(src, exe); err != nil {
		if s != nil {
			s.Close()
		}
		return fmt.Errorf("copy agent to %s: %w", exe, err)
	}
	if s != nil {
		// Reinstall: point at the installed exe and restart with the new config.
		cfg, err := s.Config()
		if err == nil {
			cfg.BinaryPathName = fmt.Sprintf(`"%s" service`, exe)
			cfg.StartType = mgr.StartAutomatic
			cfg.DelayedAutoStart = true
			err = s.UpdateConfig(cfg)
		}
		if err != nil {
			s.Close()
			return fmt.Errorf("update service: %w", err)
		}
	} else {
		s, err = m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: "Kite Agent",
			Description: "Kite monitoring agent: sends machine metrics to a Kite hub.",
			StartType:   mgr.StartAutomatic,
			// Start after boot settles, when WMI, drivers and the network are ready.
			DelayedAutoStart: true,
		}, "service")
		if err != nil {
			return fmt.Errorf("create service: %w", err)
		}
	}
	defer s.Close()
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 10 * time.Second}
	s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*60*60)
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	fmt.Printf("KiteAgent installed to %s and started\n", exe)
	return nil
}

func stopAndWait(s *mgr.Service) {
	st, err := s.Control(svc.Stop)
	if err != nil {
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for st.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return
		}
	}
}

func uninstallService() error {
	if err := requireAdmin(); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("KiteAgent is not installed")
	}
	defer s.Close()
	stopAndWait(s)
	if err := s.Delete(); err != nil {
		return err
	}
	fmt.Printf("KiteAgent removed (config kept in %s)\n", kiteDir())
	return nil
}

type handler struct{}

func (handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	if err := secureDir(kiteDir()); err == nil {
		if w, err := agent.NewRotatingFile(logPath(), 1<<20); err == nil {
			defer w.Close()
			log.SetOutput(w)
		}
	}
	if err := agent.LowerPriority(); err != nil {
		log.Printf("could not lower priority: %v", err)
	}
	cfg, err := serviceConfig(envPath())
	if err != nil {
		log.Printf("kite-agent service: %v", err)
		return true, 1 // service-specific exit code; recovery restarts at most 3 times
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	// Report Running first: detection touches DXGI and PDH, which must not
	// hold the service in StartPending.
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	go func() {
		defer close(done)
		cols := agent.Detect(cfg)
		log.Printf("kite-agent service %q sending to %s every %s (%d collectors)", cfg.Name, cfg.HubURL, cfg.Interval, len(cols))
		agent.Run(ctx, cfg, cols, agent.NewSender(cfg.HubURL, cfg.Token, 5*time.Second).Send, log.Printf)
	}()
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			cancel()
			<-done
			log.Printf("kite-agent service stopped")
			return false, 0
		}
	}
	cancel()
	return false, 0
}

func runService() error {
	return svc.Run(serviceName, handler{})
}
