package ageplugin

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--age-plugin=recipient-v1" {
		if os.Getenv("FULLA_PLUGIN_EXIT_FIXTURE") == "stubborn" {
			signal.Ignore(os.Interrupt)
			fmt.Println("ready")
			for {
				time.Sleep(time.Hour)
			}
		}
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt)
		fmt.Println("ready")
		<-interrupted
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPluginCloseReapsGracefulAndUnresponsiveChildren(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(executable, filepath.Join(dir, "age-plugin-exitfixture")); err != nil {
		t.Fatal(err)
	}
	testOnlyPluginPath = dir
	t.Cleanup(func() { testOnlyPluginPath = "" })
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	for _, mode := range []string{"graceful", "stubborn"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FULLA_PLUGIN_EXIT_FIXTURE", mode)
			cc, err := openClientConnection("exitfixture", "recipient-v1")
			if err != nil {
				t.Fatal(err)
			}
			// A watchdog bounds the fixture even if shutdown regresses to upstream's
			// unbounded Wait. It kills only the child this test created.
			watchdog := time.AfterFunc(5*time.Second, func() { _ = cc.cmd.Process.Kill() })
			defer watchdog.Stop()
			line, err := bufio.NewReader(cc.Reader).ReadString('\n')
			if err != nil || line != "ready\n" {
				t.Fatalf("fixture readiness: %q %v", line, err)
			}
			started := time.Now()
			err = cc.Close()
			elapsed := time.Since(started)
			if elapsed >= 4*time.Second {
				t.Fatalf("shutdown required watchdog: %s", elapsed)
			}
			if cc.cmd.ProcessState == nil {
				t.Fatal("plugin was not waited/reaped")
			}
			if mode == "graceful" {
				if err != nil || !cc.cmd.ProcessState.Success() {
					t.Fatalf("graceful cleanup failed: %v", err)
				}
			} else {
				ws, ok := cc.cmd.ProcessState.Sys().(syscall.WaitStatus)
				if err == nil || !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL || elapsed < time.Second {
					t.Fatalf("expected grace then SIGKILL: %v %v %s", err, ws, elapsed)
				}
			}
			if err := syscall.Kill(cc.cmd.Process.Pid, 0); err != syscall.ESRCH {
				t.Fatalf("plugin remains alive or unreaped: %v", err)
			}
		})
	}
}
