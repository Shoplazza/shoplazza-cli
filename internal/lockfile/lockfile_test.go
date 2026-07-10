package lockfile

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquire_Exclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.lock")
	rel1, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// 第二把锁（独立 fd）应超时
	if _, err := Acquire(p, 200*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	rel1()
	rel2, err := Acquire(p, time.Second) // 释放后可再取
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	rel2()
}

func TestAcquire_CreatesParentDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "locks", "deep", "b.lock")
	rel, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("acquire with missing parent: %v", err)
	}
	rel()
}
