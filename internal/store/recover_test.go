package store

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestRecoverOnlyDeadLocalOwner(t *testing.T) {
	s := fixture(t, false)
	l, err := s.Lock("test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recover(l.Token); err == nil {
		t.Fatal("stole a live lock")
	}
	// Use an actually reaped process, not an assumed-unused arbitrary PID.
	child := exec.Command("true")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	info := fmt.Sprintf("pid=%d host=%s operation=test\n", child.ProcessState.Pid(), host)
	if err := securefs.Replace(s.Root, "lock/info", []byte(info)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recover("wrong-token"); err == nil {
		t.Fatal("stole unknown owner")
	}
	result, err := s.Recover(l.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result["lock_released"] != true {
		t.Fatal(result)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
}

func TestStructuralDoctorDoesNotDecrypt(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("sample", []byte("value"), false); err != nil {
		t.Fatal(err)
	}
	c, err := s.Ciphertext("sample")
	if err != nil {
		t.Fatal(err)
	}
	c[len(c)-1] ^= 1
	if err := securefs.Replace(s.Root, "passwords/sample.age", c); err != nil {
		t.Fatal(err)
	}
	r, err := s.Doctor(false)
	if err != nil || !r.Healthy {
		t.Fatal("structural check decrypted payload", r, err)
	}
	deep, err := s.Doctor(true)
	if err != nil || deep.Healthy || len(deep.Verification) != 1 || deep.Verification[0].OK {
		t.Fatal("deep verification missed corruption", deep, err)
	}
}
