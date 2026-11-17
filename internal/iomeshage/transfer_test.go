// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

// newTestIOM returns an IOMeshage without a meshage node, enough to drive
// transfers whose parts arrive through MITM.
func newTestIOM(t *testing.T, base string) *IOMeshage {
	t.Helper()

	return &IOMeshage{
		base:      base,
		name:      "self",
		TIDs:      make(map[int64]chan *Message),
		transfers: make(map[string]*Transfer),
		failed:    make(map[string]error),
		queue:     make(chan bool, QUEUE_LEN),
		rand:      mrand.New(mrand.NewSource(1)),
		hashes:    make(map[string]string),
	}
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()

	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}

	return b
}

func infoFor(name string, data []byte, reportSize bool) *Message {
	msg := &Message{
		From:     "peer",
		Type:     TYPE_RESPONSE,
		Filename: name,
		Perm:     0640,
		Part:     (int64(len(data)) + PART_SIZE - 1) / PART_SIZE,
		ACK:      true,
	}
	if reportSize {
		msg.Size = int64(len(data))
	}

	return msg
}

func partOf(data []byte, p int64) []byte {
	end := min((p+1)*PART_SIZE, int64(len(data)))
	return data[p*PART_SIZE : end]
}

// startTransfer registers a transfer for msg the way Get does.
func startTransfer(t *testing.T, iom *IOMeshage, msg *Message) *Transfer {
	t.Helper()

	tr, err := iom.newTransfer(msg)
	if err != nil {
		t.Fatalf("newTransfer: %v", err)
	}

	iom.transferLock.Lock()
	iom.transfers[msg.Filename] = tr
	iom.transferLock.Unlock()

	return tr
}

func snoop(iom *IOMeshage, msg *Message, p int64, data []byte) {
	iom.MITM(&Message{
		From:     msg.From,
		Type:     TYPE_RESPONSE,
		Filename: msg.Filename,
		Part:     p,
		ACK:      true,
		Data:     data,
	})
}

func transferDirs(t *testing.T, base string) []string {
	t.Helper()

	dirs, err := filepath.Glob(filepath.Join(base, "transfer_*"))
	if err != nil {
		t.Fatal(err)
	}

	return dirs
}

// TestTransferReassembly fetches a multi-part file whose parts arrive out of
// order and checks the result matches byte for byte, with no leftovers.
func TestTransferReassembly(t *testing.T) {
	for _, reportSize := range []bool{true, false} {
		name := "size-reported"
		if !reportSize {
			name = "size-unknown"
		}

		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			iom := newTestIOM(t, base)

			data := randomBytes(t, 2*PART_SIZE+12345)
			msg := infoFor("sub/dir/disk.qcow2", data, reportSize)
			startTransfer(t, iom, msg)

			for _, p := range []int64{2, 0, 1} {
				snoop(iom, msg, p, partOf(data, p))
			}

			// every part is already present, so getParts only finishes the file
			iom.getParts(msg)

			got, err := os.ReadFile(filepath.Join(base, msg.Filename))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("got %v bytes that differ from the %v bytes sent", len(got), len(data))
			}

			fi, err := os.Stat(filepath.Join(base, msg.Filename))
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != msg.Perm {
				t.Errorf("got mode %v, want %v", fi.Mode().Perm(), msg.Perm)
			}

			if s := iom.Status(); len(s) != 0 {
				t.Errorf("transfer still listed: %+v", s[0])
			}
			if dirs := transferDirs(t, base); len(dirs) != 0 {
				t.Errorf("leftover transfer directories: %v", dirs)
			}
		})
	}
}

// TestTransferUsesOneCopy checks that a transfer never holds more than one
// copy of the file on disk (issue #1388).
func TestTransferUsesOneCopy(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, 3*PART_SIZE)
	msg := infoFor("big.img", data, true)
	tr := startTransfer(t, iom, msg)

	for p := int64(0); p < msg.Part; p++ {
		snoop(iom, msg, p, partOf(data, p))
	}

	entries, err := os.ReadDir(tr.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want the transfer file alone in %v, got %v entries", tr.Dir, len(entries))
	}

	iom.getParts(msg)

	got, err := os.ReadFile(filepath.Join(base, msg.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("transferred file differs")
	}
}

// TestTransferEmptyFile covers the zero-part corner case.
func TestTransferEmptyFile(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	msg := infoFor("empty", nil, true)
	startTransfer(t, iom, msg)
	iom.getParts(msg)

	fi, err := os.Stat(filepath.Join(base, "empty"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 0 {
		t.Errorf("got size %v, want 0", fi.Size())
	}
	if dirs := transferDirs(t, base); len(dirs) != 0 {
		t.Errorf("leftover transfer directories: %v", dirs)
	}
}

// TestTransferNoSpace checks that a full disk is reported when the transfer
// is created instead of truncating the file after it is downloaded.
func TestTransferNoSpace(t *testing.T) {
	orig := preallocate
	t.Cleanup(func() { preallocate = orig })

	preallocate = func(*os.File, int64) error { return syscall.ENOSPC }

	base := t.TempDir()
	iom := newTestIOM(t, base)

	_, err := iom.newTransfer(infoFor("big.img", make([]byte, PART_SIZE+1), true))
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("got %v, want ENOSPC", err)
	}
	if dirs := transferDirs(t, base); len(dirs) != 0 {
		t.Errorf("leftover transfer directories: %v", dirs)
	}

	// without a reported size nothing is reserved
	if _, err := iom.newTransfer(infoFor("big.img", make([]byte, PART_SIZE+1), false)); err != nil {
		t.Fatalf("size-unknown transfer: %v", err)
	}
}

// TestTransferNoSpaceTmpfs fills a real file system; it needs root to mount.
func TestTransferNoSpaceTmpfs(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to mount a tmpfs")
	}

	base := t.TempDir()
	if out, err := exec.Command("mount", "-t", "tmpfs", "-o", "size=24m", "tmpfs", base).CombinedOutput(); err != nil {
		t.Skipf("mount tmpfs: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("umount", base).Run() })

	iom := newTestIOM(t, base)

	_, err := iom.newTransfer(infoFor("big.img", make([]byte, 3*PART_SIZE), true))
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("got %v, want ENOSPC", err)
	}

	// a file that fits, but would not fit twice, transfers intact
	data := randomBytes(t, 2*PART_SIZE-1)
	msg := infoFor("fits.img", data, true)
	startTransfer(t, iom, msg)
	for p := int64(0); p < msg.Part; p++ {
		snoop(iom, msg, p, partOf(data, p))
	}
	iom.getParts(msg)

	got, err := os.ReadFile(filepath.Join(base, msg.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %v bytes that differ from the %v bytes sent", len(got), len(data))
	}
}

// TestMITMRejectsBadParts checks that empty, short, or out of range parts are
// not recorded as received, so they cannot silently corrupt the file.
func TestMITMRejectsBadParts(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, 2*PART_SIZE+10)
	msg := infoFor("f", data, true)
	tr := startTransfer(t, iom, msg)

	snoop(iom, msg, 0, nil)                       // empty
	snoop(iom, msg, 1, data[:PART_SIZE-1])        // short middle part
	snoop(iom, msg, 2, data[:5])                  // wrong final length
	snoop(iom, msg, 3, partOf(data, 2))           // out of range
	snoop(iom, msg, -1, partOf(data, 0))          // negative
	snoop(iom, msg, 0, make([]byte, PART_SIZE+1)) // too long

	if len(tr.Parts) != 0 {
		t.Fatalf("bad parts recorded: %v", tr.Parts)
	}

	snoop(iom, msg, 2, partOf(data, 2))
	if !tr.Parts[2] {
		t.Fatal("valid part not recorded")
	}
}

func TestCheckPart(t *testing.T) {
	full := make([]byte, PART_SIZE)

	tests := []struct {
		name     string
		p        int64
		n        int
		numParts int64
		size     int64
		ok       bool
	}{
		{"full middle part", 0, PART_SIZE, 3, 0, true},
		{"short middle part", 1, PART_SIZE - 1, 3, 0, false},
		{"last part, size unknown", 2, 7, 3, 0, true},
		{"last part, size matches", 2, 7, 3, 2*PART_SIZE + 7, true},
		{"last part, size mismatch", 2, 8, 3, 2*PART_SIZE + 7, false},
		{"last part exactly full", 1, PART_SIZE, 2, 2 * PART_SIZE, true},
		{"empty part", 0, 0, 1, 0, false},
		{"beyond last part", 3, PART_SIZE, 3, 0, false},
		{"negative part", -1, PART_SIZE, 3, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPart(tt.p, full[:tt.n], tt.numParts, tt.size)
			if (err == nil) != tt.ok {
				t.Errorf("checkPart(%v, %v bytes, %v, %v) = %v, want ok=%v", tt.p, tt.n, tt.numParts, tt.size, err, tt.ok)
			}
		})
	}
}

// TestTransferReadPart checks that in-flight parts are served from the
// transfer file with their original length.
func TestTransferReadPart(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, PART_SIZE+99)
	msg := infoFor("f", data, true)
	tr := startTransfer(t, iom, msg)

	snoop(iom, msg, 1, partOf(data, 1))

	got, err := tr.readPart(1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, partOf(data, 1)) {
		t.Fatalf("got %v bytes, want %v", len(got), len(partOf(data, 1)))
	}
}

// TestReadPart covers serving parts of a complete local file.
func TestReadPart(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, PART_SIZE+3)
	if err := os.WriteFile(filepath.Join(base, "f"), data, 0644); err != nil {
		t.Fatal(err)
	}

	for p := int64(0); p < 2; p++ {
		got, err := iom.readPart("f", p)
		if err != nil {
			t.Fatalf("part %v: %v", p, err)
		}
		if !bytes.Equal(got, partOf(data, p)) {
			t.Errorf("part %v: got %v bytes, want %v", p, len(got), len(partOf(data, p)))
		}
	}

	if _, err := iom.readPart("f", 2); err == nil {
		t.Error("reading one part past the end succeeded")
	}
	if _, err := iom.readPart("missing", 0); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Errorf("reading a missing file: got %v", err)
	}
}

// TestInfoResponse checks that info replies carry the file size, so the
// requester can reserve space, and never offer in-progress transfers.
func TestInfoResponse(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, PART_SIZE+7)
	if err := os.MkdirAll(filepath.Join(base, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "dir", "f"), data, 0640); err != nil {
		t.Fatal(err)
	}

	resp := iom.infoResponse(&Message{From: "peer", Filename: "dir/f", TID: 9})
	if !resp.ACK || resp.Size != int64(len(data)) || resp.Part != 2 || resp.TID != 9 || resp.From != "self" {
		t.Errorf("file: got ACK=%v Size=%v Part=%v TID=%v From=%v", resp.ACK, resp.Size, resp.Part, resp.TID, resp.From)
	}
	if resp.Perm != 0640 {
		t.Errorf("file: got Perm=%v, want 0640", resp.Perm)
	}

	// a transfer in progress next to it must not show up in a directory or glob
	startTransfer(t, iom, infoFor("dir/g", data, true))

	for _, name := range []string{"/", "*", "dir"} {
		resp := iom.infoResponse(&Message{From: "peer", Filename: name})
		for _, f := range resp.Glob {
			if strings.HasPrefix(f, transferDirPrefix) {
				t.Errorf("%v: glob offers in-progress transfer %v", name, f)
			}
		}
	}

	if resp := iom.infoResponse(&Message{From: "peer", Filename: "missing"}); resp.ACK {
		t.Error("missing file: got ACK")
	}
}

// TestPartResponse covers serving parts of complete and in-flight files,
// including NACKs instead of empty data when a part cannot be read.
func TestPartResponse(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, PART_SIZE+7)
	if err := os.WriteFile(filepath.Join(base, "full"), data, 0644); err != nil {
		t.Fatal(err)
	}

	resp := iom.partResponse(&Message{From: "peer", Filename: "full", Part: 1}, true)
	if !resp.ACK || !bytes.Equal(resp.Data, partOf(data, 1)) {
		t.Errorf("complete file part 1: ACK=%v, %v bytes", resp.ACK, len(resp.Data))
	}
	if resp := iom.partResponse(&Message{From: "peer", Filename: "full", Part: 2}, true); resp.ACK || resp.Data != nil {
		t.Errorf("part past the end: ACK=%v, %v bytes", resp.ACK, len(resp.Data))
	}

	msg := infoFor("partial", data, true)
	startTransfer(t, iom, msg)
	snoop(iom, msg, 1, partOf(data, 1))

	resp = iom.partResponse(&Message{From: "peer", Filename: "partial", Part: 1}, true)
	if !resp.ACK || !bytes.Equal(resp.Data, partOf(data, 1)) {
		t.Errorf("in-flight part 1: ACK=%v, %v bytes", resp.ACK, len(resp.Data))
	}
	if resp := iom.partResponse(&Message{From: "peer", Filename: "partial", Part: 1}, false); !resp.ACK || resp.Data != nil {
		t.Errorf("whohas in-flight part 1: ACK=%v, %v bytes", resp.ACK, len(resp.Data))
	}
	if resp := iom.partResponse(&Message{From: "peer", Filename: "partial", Part: 0}, true); resp.ACK {
		t.Error("in-flight part 0 not received yet: got ACK")
	}
}

// TestTransferFailureReported checks that a transfer that fails after Get
// returned is reported to waiters instead of only being logged.
func TestTransferFailureReported(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	// a regular file where the destination directory should be
	if err := os.WriteFile(filepath.Join(base, "blocker"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	data := randomBytes(t, PART_SIZE+1)
	msg := infoFor("blocker/disk.img", data, true)
	startTransfer(t, iom, msg)
	for p := int64(0); p < msg.Part; p++ {
		snoop(iom, msg, p, partOf(data, p))
	}
	iom.getParts(msg)

	empty := infoFor("blocker/empty", nil, true)
	startTransfer(t, iom, empty)
	iom.getParts(empty)

	for _, name := range []string{"blocker/disk.img", "blocker", "blocker/"} {
		if err := iom.Failed(name); err == nil || !strings.Contains(err.Error(), "disk.img") {
			t.Errorf("Failed(%q) = %v, want the disk.img failure", name, err)
		}
	}
	if err := iom.Failed("blocker/empty"); err == nil {
		t.Error("empty file failure not reported")
	}
	if err := iom.Failed("block"); err != nil {
		t.Errorf("Failed of a name prefix matched another file: %v", err)
	}
	if dirs := transferDirs(t, base); len(dirs) != 0 {
		t.Errorf("leftover transfer directories: %v", dirs)
	}

	iom.clearFailed("blocker")
	if err := iom.Failed("blocker"); err != nil {
		t.Errorf("failure still reported after clearFailed: %v", err)
	}
}

// TestRemoveStaleTransfers checks that startup removes this node's leftover
// transfer directories, and only this node's. Files and directories that only
// look like transfer directories stay.
func TestRemoveStaleTransfers(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	tr, err := iom.newTransfer(infoFor("f", []byte("x"), true))
	if err != nil {
		t.Fatal(err)
	}

	mine := filepath.Join(base, iom.transferPrefix()+"123")
	other := filepath.Join(base, transferDirPrefix+"other_456")
	longer := filepath.Join(base, transferDirPrefix+"self_b_789") // node "self_b"
	for _, dir := range []string{mine, other, longer} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	userFiles := []string{"transfer_self_42", "transfer_self_backup.qcow2", "transfer_self_x/f"}
	for _, f := range userFiles {
		path := filepath.Join(base, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	iom.removeStaleTransfers()

	for _, dir := range []string{tr.Dir, mine} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("own stale transfer %v still there: %v", dir, err)
		}
	}
	for _, dir := range []string{other, longer} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("another node's transfer was removed: %v", err)
		}
	}

	files, err := iom.List("/", true)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, f := range files {
		got = append(got, iom.Rel(f))
	}
	if !slices.Equal(got, userFiles) {
		t.Errorf("List shows %v, want only %v", got, userFiles)
	}
}

// TestTransferNamedFilesServed checks that files and directories whose names
// only start like a transfer directory are listed and served.
func TestTransferNamedFilesServed(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, 10)
	for _, f := range []string{"transfer_disk.qcow2", "transfer_dir/f"} {
		path := filepath.Join(base, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}

		if resp := iom.infoResponse(&Message{From: "peer", Filename: f}); !resp.ACK || resp.Size != int64(len(data)) {
			t.Errorf("%v: got ACK=%v Size=%v", f, resp.ACK, resp.Size)
		}
	}

	resp := iom.infoResponse(&Message{From: "peer", Filename: "transfer_*"})
	if !slices.Contains(resp.Glob, "transfer_disk.qcow2") || !slices.Contains(resp.Glob, "transfer_dir/f") {
		t.Errorf("glob: got %v", resp.Glob)
	}
}

// TestQueuedTransfersHoldNoFiles checks that transfers waiting in the queue
// keep no file open, so a large directory cannot use up file descriptors.
func TestQueuedTransfersHoldNoFiles(t *testing.T) {
	fds := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skipf("cannot count open files: %v", err)
		}
		return len(entries)
	}

	base := t.TempDir()
	iom := newTestIOM(t, base)

	before := fds()
	for i := 0; i < 50; i++ {
		data := []byte("x")
		startTransfer(t, iom, infoFor(fmt.Sprintf("dir/f%v", i), data, true))
	}
	if after := fds(); after-before >= 50 {
		t.Errorf("open files went from %v to %v for 50 queued transfers", before, after)
	}
}

// TestTransferFileMode checks that the transfer file is private while it is
// written, and that it has its final mode when it appears.
func TestTransferFileMode(t *testing.T) {
	base := t.TempDir()
	iom := newTestIOM(t, base)

	data := randomBytes(t, 10)
	msg := infoFor("f", data, true)
	msg.Perm = 0604
	tr := startTransfer(t, iom, msg)

	fi, err := os.Stat(tr.path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("transfer file has mode %v, want 0600", fi.Mode().Perm())
	}

	snoop(iom, msg, 0, data)
	if err := iom.finishTransfer(tr, msg.Perm); err != nil {
		t.Fatal(err)
	}

	fi, err = os.Stat(filepath.Join(base, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != msg.Perm {
		t.Errorf("got mode %v, want %v", fi.Mode().Perm(), msg.Perm)
	}

	// the final mode is set before the file is moved into place
	if err := os.MkdirAll(filepath.Join(base, "blocked", "x"), 0755); err != nil {
		t.Fatal(err)
	}
	blocked := infoFor("blocked", data, true)
	blocked.Perm = 0604
	tr = startTransfer(t, iom, blocked)
	snoop(iom, blocked, 0, data)
	if err := iom.finishTransfer(tr, blocked.Perm); err == nil {
		t.Fatal("moving the file onto a directory succeeded")
	}

	fi, err = os.Stat(tr.path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != blocked.Perm {
		t.Errorf("transfer file has mode %v before the move, want %v", fi.Mode().Perm(), blocked.Perm)
	}
}
