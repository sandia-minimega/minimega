// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// newTestHasher makes a hasher over base with a short hashDelay, but does not
// start it. Change its hash function before startTestHasher, not after.
func newTestHasher(t *testing.T, base string) (*IOMeshage, *hasher) {
	t.Helper()

	old := hashDelay
	hashDelay = 100 * time.Millisecond
	t.Cleanup(func() { hashDelay = old })

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { watcher.Close() })

	iom := &IOMeshage{base: base, hash: true, hashes: make(map[string]string)}

	return iom, newHasher(iom, watcher)
}

// startTestHasher starts h the way startHasher does, and waits until the base
// directory is watched. Files created after that are seen by the hasher.
func startTestHasher(t *testing.T, h *hasher) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.start()
	}()

	t.Cleanup(func() {
		h.watcher.Close()
		<-done
	})

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if slices.Contains(h.watcher.WatchList(), h.iom.base) {
			return
		}
	}

	t.Fatal("base directory is not watched")
}

// testHasher runs a hasher over base with a short hashDelay.
func testHasher(t *testing.T, base string) (*IOMeshage, *hasher) {
	t.Helper()

	iom, h := newTestHasher(t, base)
	startTestHasher(t, h)

	return iom, h
}

// blockHash makes the hash of path wait until release is closed, and then
// return result. started receives a value when that hash starts. Other paths
// are hashed as usual.
func blockHash(h *hasher, path, result string) (started, release chan struct{}) {
	started = make(chan struct{}, 16)
	release = make(chan struct{})

	hash := h.hash
	h.hash = func(ctx context.Context, p string) (string, error) {
		if p != path {
			return hash(ctx, p)
		}

		started <- struct{}{}
		<-release

		return result, nil
	}

	return started, release
}

// waitIdle waits until no hash is running.
func waitIdle(t *testing.T, h *hasher) {
	t.Helper()

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if len(h.sem) == 0 {
			return
		}
	}

	t.Fatal("a hash is still running")
}

func waitChan(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-c:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %v", what)
	}
}

// countHashes wraps the hasher's hash function to count how often it runs.
func countHashes(h *hasher) *atomic.Int64 {
	var n atomic.Int64

	hash := h.hash
	h.hash = func(ctx context.Context, path string) (string, error) {
		n.Add(1)
		return hash(ctx, path)
	}

	return &n
}

func mustHash(t *testing.T, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "want")
	mustWrite(t, path, data)

	hash, err := hashFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	return hash
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// waitHash waits for path to have the wanted hash ("" for none).
func waitHash(t *testing.T, iom *IOMeshage, path, want string) {
	t.Helper()

	var got string

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got = iom.getHash(path); got == want {
			return
		}
	}

	t.Fatalf("hash of %v = %q, want %q", path, got, want)
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	mustWrite(t, path, []byte("hello world"))

	hash, err := hashFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 16 {
		t.Errorf("hash = %q, want 16 hex digits", hash)
	}

	// same content, same hash; a symlink is hashed as its target
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if got, err := hashFile(context.Background(), link); err != nil || got != hash {
		t.Errorf("hash via symlink = %q, %v, want %q", got, err, hash)
	}

	mustWrite(t, path, []byte("hello world!"))
	if got, err := hashFile(context.Background(), path); err != nil || got == hash {
		t.Errorf("hash of different content = %q, %v, want it to differ from %q", got, err, hash)
	}

	if _, err := hashFile(context.Background(), filepath.Join(dir, "missing")); err == nil {
		t.Error("hashing a missing file did not fail")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hashFile(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("hashing with a cancelled context: %v, want context.Canceled", err)
	}
}

func TestIgnoreDirectory(t *testing.T) {
	tests := map[string]bool{
		"/files/transfer_123":          true,
		"/files/transfer_123/foo.part": true,
		"/files/foo.qc2":               false,
		"/files/my_transfer_notes.txt": false,
		"/files/images/file_transfer_": false,

		// every component is matched, including the file name
		"/files/transfer_notes.img":    true,
		"/files/my_transfer_notes.img": false,
		"/transfer_area/files/foo.img": true,
	}

	for path, want := range tests {
		if got := ignoreDirectory(path); got != want {
			t.Errorf("ignoreDirectory(%q) = %v, want %v", path, got, want)
		}
	}
}

// Files that exist when the hasher starts are hashed, except the ones that
// never were: empty files, symlinks and in-flight transfers.
func TestHasherExistingFiles(t *testing.T) {
	base := t.TempDir()

	mustWrite(t, filepath.Join(base, "top"), []byte("top"))
	mustWrite(t, filepath.Join(base, "a", "b", "nested"), []byte("nested"))
	mustWrite(t, filepath.Join(base, "empty"), nil)
	mustWrite(t, filepath.Join(base, "transfer_1", "part"), []byte("part"))
	if err := os.Symlink(filepath.Join(base, "top"), filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}

	iom, _ := testHasher(t, base)

	waitHash(t, iom, filepath.Join(base, "top"), mustHash(t, []byte("top")))
	waitHash(t, iom, filepath.Join(base, "a", "b", "nested"), mustHash(t, []byte("nested")))

	// relative paths are looked up below the base directory
	if got := iom.getHash("a/b/nested"); got != mustHash(t, []byte("nested")) {
		t.Errorf("hash by relative path = %q", got)
	}

	for _, f := range []string{"empty", "link", "transfer_1/part"} {
		if got := iom.getHash(f); got != "" {
			t.Errorf("%v was hashed: %q", f, got)
		}
	}
}

// A file written in many small chunks is hashed once, after the last write,
// not once per write (sandia-minimega/minimega#1677).
func TestHasherHashesOnceAfterWrites(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)
	runs := countHashes(h)
	startTestHasher(t, h)

	path := filepath.Join(base, "big.img")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	chunk := bytes.Repeat([]byte("x"), 4096)

	var data []byte

	// writes are closer together than hashDelay, like a copy in progress
	for i := 0; i < 50; i++ {
		if _, err := f.Write(chunk); err != nil {
			t.Fatal(err)
		}

		data = append(data, chunk...)
		time.Sleep(hashDelay / 10)

		if got := iom.getHash(path); got != "" {
			t.Fatalf("file has hash %q while it is still being written", got)
		}
	}

	waitHash(t, iom, path, mustHash(t, data))

	// give any extra hashes a chance to show up
	time.Sleep(3 * hashDelay)

	if n := runs.Load(); n != 1 {
		t.Errorf("file was hashed %d times, want 1", n)
	}
}

// A hash in progress is abandoned when the file changes underneath it, and
// its result never replaces the hash of the newer content.
func TestHasherChangeWhileHashing(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	started := make(chan struct{}, 16)
	cancelled := make(chan struct{}, 16)

	var block atomic.Bool
	block.Store(true)

	hash := h.hash
	h.hash = func(ctx context.Context, path string) (string, error) {
		if block.Load() {
			started <- struct{}{}
			<-ctx.Done()
			cancelled <- struct{}{}

			// a slow hash of the old content finishing late
			return "stale", nil
		}

		return hash(ctx, path)
	}

	startTestHasher(t, h)

	path := filepath.Join(base, "file")
	mustWrite(t, path, []byte("old"))

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("file was never hashed")
	}

	block.Store(false)
	mustWrite(t, path, []byte("new content"))

	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("hash in progress was not cancelled when the file changed")
	}

	waitHash(t, iom, path, mustHash(t, []byte("new content")))
}

func TestHasherRemoveAndRename(t *testing.T) {
	base := t.TempDir()
	iom, h := testHasher(t, base)

	want := mustHash(t, []byte("data"))

	file := filepath.Join(base, "file")
	mustWrite(t, file, []byte("data"))
	waitHash(t, iom, file, want)

	// renaming a file moves its hash
	moved := filepath.Join(base, "moved")
	if err := os.Rename(file, moved); err != nil {
		t.Fatal(err)
	}
	waitHash(t, iom, moved, want)
	waitHash(t, iom, file, "")

	if err := os.Remove(moved); err != nil {
		t.Fatal(err)
	}
	waitHash(t, iom, moved, "")

	// renaming a directory moves the hashes of the files in it
	inDir := filepath.Join(base, "dir", "sub", "file")
	mustWrite(t, inDir, []byte("data"))
	waitHash(t, iom, inDir, want)

	if err := os.Rename(filepath.Join(base, "dir"), filepath.Join(base, "dir2")); err != nil {
		t.Fatal(err)
	}
	waitHash(t, iom, filepath.Join(base, "dir2", "sub", "file"), want)
	waitHash(t, iom, inDir, "")

	// and files added to the renamed directory are hashed under its new name
	added := filepath.Join(base, "dir2", "sub", "added")
	mustWrite(t, added, []byte("data"))
	waitHash(t, iom, added, want)

	// a file removed before it was hashed gets no hash
	gone := filepath.Join(base, "gone")
	mustWrite(t, gone, []byte("data"))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * hashDelay)
	if got := iom.getHash(gone); got != "" {
		t.Errorf("removed file has hash %q", got)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.jobs) != 0 {
		t.Errorf("%d hash jobs left behind", len(h.jobs))
	}
}

// A timer that fired for a job that has since been replaced must not hash on
// behalf of the new job, whose quiet period has not passed yet.
func TestHasherStaleTimer(t *testing.T) {
	base := t.TempDir()
	_, h := newTestHasher(t, base)
	runs := countHashes(h)

	path := filepath.Join(base, "file")
	mustWrite(t, path, []byte("data"))

	stale := &hashJob{gen: 1}
	h.jobs[path] = &hashJob{gen: 1}

	h.run(path, stale, 1)

	if n := runs.Load(); n != 0 {
		t.Errorf("stale timer hashed the file %d times", n)
	}
	if h.jobs[path] == nil {
		t.Error("stale timer removed the current job")
	}
}

// A timer that fired for an older generation of the current job does not hash
// the file, because the file changed again since then.
func TestHasherOldGeneration(t *testing.T) {
	base := t.TempDir()
	_, h := newTestHasher(t, base)
	runs := countHashes(h)

	path := filepath.Join(base, "file")
	mustWrite(t, path, []byte("data"))

	job := &hashJob{gen: 2}
	h.jobs[path] = job

	h.run(path, job, 1)

	if n := runs.Load(); n != 0 {
		t.Errorf("timer of an old generation hashed the file %d times", n)
	}
	if h.jobs[path] != job {
		t.Error("timer of an old generation removed the job")
	}
}

// Forgetting a file stops its pending hash.
func TestHasherForgetFile(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)
	runs := countHashes(h)

	path := filepath.Join(base, "file")
	mustWrite(t, path, []byte("data"))

	h.schedule(path)
	h.forget(path)

	time.Sleep(3 * hashDelay)

	if n := runs.Load(); n != 0 {
		t.Errorf("forgotten file was hashed %d times", n)
	}
	if got := iom.getHash(path); got != "" {
		t.Errorf("forgotten file has hash %q", got)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.jobs[path] != nil {
		t.Error("forgotten file still has a hash job")
	}
}

// Files that land in a directory right after it is created are hashed even
// though the directory is not being watched yet.
func TestHasherNewDirectoryWithContent(t *testing.T) {
	base := t.TempDir()
	iom, _ := testHasher(t, base)

	want := mustHash(t, []byte("data"))

	for i := 0; i < 20; i++ {
		path := filepath.Join(base, "new", string(rune('a'+i)), "deep", "file")
		mustWrite(t, path, []byte("data"))
	}

	for i := 0; i < 20; i++ {
		waitHash(t, iom, filepath.Join(base, "new", string(rune('a'+i)), "deep", "file"), want)
	}
}

// No more than hashWorkers files are hashed at the same time.
func TestHasherWorkerLimit(t *testing.T) {
	base := t.TempDir()

	const files = 32

	for i := 0; i < files; i++ {
		mustWrite(t, filepath.Join(base, string(rune('a'+i))), []byte("data"))
	}

	iom, h := newTestHasher(t, base)

	var (
		mu           sync.Mutex
		running, max int
	)

	hash := h.hash
	h.hash = func(ctx context.Context, path string) (string, error) {
		mu.Lock()
		running++
		if running > max {
			max = running
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		running--
		mu.Unlock()

		return hash(ctx, path)
	}

	startTestHasher(t, h)

	want := mustHash(t, []byte("data"))
	for i := 0; i < files; i++ {
		waitHash(t, iom, filepath.Join(base, string(rune('a'+i))), want)
	}

	mu.Lock()
	defer mu.Unlock()

	if max > hashWorkers {
		t.Errorf("%d files hashed at once, limit is %d", max, hashWorkers)
	}
}

// Directories created and removed while the existing files are scanned do not
// race with the scan, and files created during the scan are hashed.
func TestHasherStartWhileChanging(t *testing.T) {
	base := t.TempDir()

	for i := 0; i < 300; i++ {
		if err := os.MkdirAll(filepath.Join(base, fmt.Sprintf("d%03d", i), "x"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// the scan of the 300 directories can still be running when this returns
	iom, _ := testHasher(t, base)

	for i := 0; i < 100; i++ {
		mustWrite(t, filepath.Join(base, fmt.Sprintf("new%03d", i), "file"), []byte("data"))

		gone := filepath.Join(base, fmt.Sprintf("gone%03d", i))
		if err := os.Mkdir(gone, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(gone); err != nil {
			t.Fatal(err)
		}
	}

	want := mustHash(t, []byte("data"))
	for i := 0; i < 100; i++ {
		waitHash(t, iom, filepath.Join(base, fmt.Sprintf("new%03d", i), "file"), want)
	}
}

// Rewriting a hashed file drops its hash until the new contents are hashed.
func TestHasherRewriteClearsHash(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	var block atomic.Bool
	started := make(chan struct{}, 16)
	release := make(chan struct{})

	hash := h.hash
	h.hash = func(ctx context.Context, path string) (string, error) {
		if block.Load() {
			started <- struct{}{}
			<-release
		}

		return hash(ctx, path)
	}

	startTestHasher(t, h)

	path := filepath.Join(base, "file")
	mustWrite(t, path, []byte("one"))
	waitHash(t, iom, path, mustHash(t, []byte("one")))

	// hold the hash of the new contents, so the hash stays cleared while it is checked
	block.Store(true)
	mustWrite(t, path, []byte("two"))
	waitChan(t, started, "the hash to start")

	if got := iom.getHash(path); got != "" {
		t.Errorf("hash of rewritten file = %q before it is hashed again, want none", got)
	}

	block.Store(false)
	close(release)
	waitHash(t, iom, path, mustHash(t, []byte("two")))
}

// A file that changed, but cannot be read, has no hash.
func TestHasherUnreadableClearsHash(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the file")
	}

	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	dir := filepath.Join(base, "dir")
	path := filepath.Join(dir, "file")
	mustWrite(t, path, []byte("data"))
	iom.updateHash(path, "old")

	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	h.schedule(path)

	if got := iom.getHash(path); got != "" {
		t.Errorf("hash of unreadable file = %q, want none", got)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.jobs[path] != nil {
		t.Error("a hash was scheduled for an unreadable file")
	}
}

// An empty file or a symlink moved over a hashed file drops its hash. On Linux
// the hashed name gets only a CREATE event.
func TestHasherReplaceWithUnhashed(t *testing.T) {
	base := t.TempDir()
	iom, _ := testHasher(t, base)

	want := mustHash(t, []byte("data"))

	file := filepath.Join(base, "file")
	mustWrite(t, file, []byte("data"))
	waitHash(t, iom, file, want)

	empty := filepath.Join(base, "empty")
	mustWrite(t, empty, nil)
	if err := os.Rename(empty, file); err != nil {
		t.Fatal(err)
	}
	waitHash(t, iom, file, "")

	other := filepath.Join(base, "other")
	mustWrite(t, other, []byte("data"))
	waitHash(t, iom, other, want)

	link := filepath.Join(base, "link")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target")
	mustWrite(t, target, []byte("data"))
	waitHash(t, iom, target, want)

	if err := os.Rename(link, target); err != nil {
		t.Fatal(err)
	}
	waitHash(t, iom, target, "")
}

// A FIFO is never hashed: opening it would block a hash worker until a writer
// opens it, and stop all hashing when hashWorkers is 1.
func TestHasherSkipsFIFO(t *testing.T) {
	old := hashWorkers
	hashWorkers = 1
	t.Cleanup(func() { hashWorkers = old })

	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	fifo := filepath.Join(base, "fifo")
	if err := syscall.Mkfifo(fifo, 0644); err != nil {
		t.Fatal(err)
	}

	// release a hash blocked on the FIFO, if there is one
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
	})

	file := filepath.Join(base, "file")
	mustWrite(t, file, []byte("data"))

	h.schedule(fifo)
	h.schedule(file)

	h.mu.Lock()
	job := h.jobs[fifo]
	h.mu.Unlock()

	if job != nil {
		t.Error("a hash was scheduled for a FIFO")
	}

	waitHash(t, iom, file, mustHash(t, []byte("data")))
}

// Files below a directory that cannot be watched are still hashed.
func TestHasherUnwatchableDirectory(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	mustWrite(t, filepath.Join(base, "file"), []byte("data"))
	mustWrite(t, filepath.Join(base, "sub", "nested"), []byte("data"))

	// every Add fails on a closed watcher
	h.watcher.Close()
	h.addDir(base)

	want := mustHash(t, []byte("data"))
	waitHash(t, iom, filepath.Join(base, "file"), want)
	waitHash(t, iom, filepath.Join(base, "sub", "nested"), want)

	if len(h.dirs) != 0 {
		t.Errorf("directories that are not watched are in dirs: %v", h.dirs)
	}
}

// A hash that finishes after its file was removed and scheduled again must not
// replace the hash of the new job, which also has generation 1.
func TestHasherReplacedJob(t *testing.T) {
	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	path := filepath.Join(base, "file")
	started, release := blockHash(h, path, "stale")
	mustWrite(t, path, []byte("data"))

	h.schedule(path)
	waitChan(t, started, "the hash to start")

	// the new job must not be hashed while the test runs
	hashDelay = time.Hour

	h.forget(path)
	h.schedule(path)

	h.mu.Lock()
	job := h.jobs[path]
	h.mu.Unlock()

	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		job.stop()
	})

	close(release)
	waitIdle(t, h)

	if got := iom.getHash(path); got == "stale" {
		t.Error("the replaced job set the hash")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.jobs[path] != job {
		t.Error("the replaced job removed the new job")
	}
}

// A hash of a file that finishes after its directory was renamed does not set
// a hash for the old path.
func TestHasherRenameDirWhileHashing(t *testing.T) {
	// the blocked hash must not stop the files under the new name from hashing
	old := hashWorkers
	hashWorkers = 2
	t.Cleanup(func() { hashWorkers = old })

	base := t.TempDir()
	iom, h := newTestHasher(t, base)

	path := filepath.Join(base, "dir", "sub", "file")
	started, release := blockHash(h, path, mustHash(t, []byte("data")))

	startTestHasher(t, h)
	mustWrite(t, path, []byte("data"))
	waitChan(t, started, "the hash to start")

	if err := os.Rename(filepath.Join(base, "dir"), filepath.Join(base, "dir2")); err != nil {
		t.Fatal(err)
	}

	waitHash(t, iom, filepath.Join(base, "dir2", "sub", "file"), mustHash(t, []byte("data")))

	// give the rename time to drop the job of the old path
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		h.mu.Lock()
		job := h.jobs[path]
		h.mu.Unlock()

		if job == nil {
			break
		}
	}

	close(release)
	waitIdle(t, h)

	if got := iom.getHash(path); got != "" {
		t.Errorf("hash of %v after its directory was renamed = %q, want none", path, got)
	}
}

// Renaming a directory keeps the hashes of a directory whose name starts with
// the same string.
func TestHasherRenameDirKeepsSibling(t *testing.T) {
	base := t.TempDir()
	iom, _ := testHasher(t, base)

	want := mustHash(t, []byte("data"))

	file := filepath.Join(base, "dir", "f")
	sibling := filepath.Join(base, "dir2", "f")
	mustWrite(t, file, []byte("data"))
	mustWrite(t, sibling, []byte("data"))
	waitHash(t, iom, file, want)
	waitHash(t, iom, sibling, want)

	if err := os.Rename(filepath.Join(base, "dir"), filepath.Join(base, "dir3")); err != nil {
		t.Fatal(err)
	}

	waitHash(t, iom, file, "")
	waitHash(t, iom, filepath.Join(base, "dir3", "f"), want)

	if got := iom.getHash(sibling); got != want {
		t.Errorf("hash of %v = %q after renaming %v, want %q", sibling, got, filepath.Dir(file), want)
	}
}

// A relative base directory must not change which hash a file name maps to.
func TestNewRelativeBase(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	iom, err := New("files", nil, "", false)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(dir, "files"); iom.base != want {
		// TempDir may itself be reached through a symlink
		if real, _ := filepath.EvalSymlinks(want); iom.base != real {
			t.Fatalf("base = %q, want %q", iom.base, want)
		}
	}

	// the hasher stores hashes by the path it watches, Get looks them up by name
	iom.updateHash(filepath.Join(iom.base, "foo.qc2"), "abc")
	if got := iom.getHash("foo.qc2"); got != "abc" {
		t.Errorf("hash by name = %q, want abc", got)
	}

	iom.updateHash(filepath.Join(iom.base, "dir", "bar"), "def")
	iom.deleteHashes("dir")
	if got := iom.getHash("dir/bar"); got != "" {
		t.Errorf("hash survived deleting its directory: %q", got)
	}
	if got := iom.getHash("foo.qc2"); got != "abc" {
		t.Errorf("deleting a directory dropped an unrelated hash")
	}
}
