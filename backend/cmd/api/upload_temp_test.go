package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestUploadTempCrashRecoveryPreservesOtherProcess(t *testing.T) {
	if parent := os.Getenv("ANBY_UPLOAD_TEMP_TEST_PARENT"); parent != "" {
		path, _, err := prepareUploadTemp(parent)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "multipart-active"), []byte("active"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, "ready"), []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	parent := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestUploadTempCrashRecoveryPreservesOtherProcess$")
	child.Env = append(os.Environ(), "ANBY_UPLOAD_TEMP_TEST_PARENT="+parent)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	var active []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		active, _ = os.ReadFile(filepath.Join(parent, "ready"))
		if len(active) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(active) == 0 {
		t.Fatal("child did not initialize upload directory")
	}
	_, cleanup, err := prepareUploadTemp(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(string(active), "multipart-active")); err != nil {
		t.Fatalf("another process's upload was removed: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	_, cleanupRecovered, err := prepareUploadTemp(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupRecovered()
	if _, err := os.Stat(string(active)); !os.IsNotExist(err) {
		t.Fatalf("crashed process directory was not reclaimed: %v", err)
	}
}
