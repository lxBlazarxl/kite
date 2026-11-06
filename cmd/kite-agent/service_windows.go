package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(kiteDir(), 0o755); err != nil {
		return err
	}
	if err := writePrivate(envPath(), env); err != nil {
		return fmt.Errorf("write %s: %w", envPath(), err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		// Reinstall: point at this exe and restart with the new config.
		stopAndWait(s)
		cfg, err := s.Config()
		if err == nil {
			cfg.BinaryPathName = fmt.Sprintf(`"%s" service`, exe)
			s.UpdateConfig(cfg)
		}
	} else {
		s, err = m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: "Kite Agent",
			Description: "Kite monitoring agent: sends machine metrics to a Kite hub.",
			StartType:   mgr.StartAutomatic,
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
	fmt.Println("KiteAgent installed and started")
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
	if err := os.MkdirAll(kiteDir(), 0o755); err == nil {
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
	cols := agent.Detect(cfg)
	log.Printf("kite-agent service %q sending to %s every %s (%d collectors)", cfg.Name, cfg.HubURL, cfg.Interval, len(cols))
	go func() {
		defer close(done)
		agent.Run(ctx, cfg, cols, agent.NewSender(cfg.HubURL, cfg.Token, 5*time.Second).Send, log.Printf)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
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
