// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// UPDATE_TIMEOUT is the number of seconds that the server holds commands for a
// client after it sends the client an update.
const UPDATE_TIMEOUT = 120

var (
	// validPlatform matches the GOOS and GOARCH values that a client can report.
	validPlatform = regexp.MustCompile(`^[a-z0-9_]+$`)

	updateFileName = regexp.MustCompile(`^miniccc(-[a-z0-9_]+-[a-z0-9_]+)?(\.exe)?$`)
)

// stagedHash is a cached SHA-256 hash of a staged update binary.
type stagedHash struct {
	size    int64
	modTime time.Time
	sum     string
}

// IsUpdateFile reports whether name is the name of a miniccc binary that the
// server can send as an update.
func IsUpdateFile(name string) bool {
	return updateFileName.MatchString(name)
}

// updateFileNames returns the names of the files that the server can send to
// a client with the given OS and architecture, in the order that it looks for
// them. The generic name is only used when the architecture is the same as the
// architecture of the server.
func updateFileNames(goos, goarch string) []string {
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	names := []string{fmt.Sprintf("miniccc-%v-%v%v", goos, goarch, ext)}
	if goarch == runtime.GOARCH {
		names = append(names, "miniccc"+ext)
	}

	return names
}

// updateFilePath finds the staged update binary for a client. It looks in the
// subpath first and then in the files directory for each name.
func (s *Server) updateFilePath(goos, goarch string) (string, error) {
	if !validPlatform.MatchString(goos) || !validPlatform.MatchString(goarch) {
		return "", fmt.Errorf("invalid client platform %q/%q", goos, goarch)
	}

	names := updateFileNames(goos, goarch)
	for _, name := range names {
		for _, dir := range []string{filepath.Join(s.path, s.subpath), s.path} {
			fpath := filepath.Join(dir, name)
			if fi, err := os.Stat(fpath); err == nil && fi.Mode().IsRegular() {
				return fpath, nil
			}
		}
	}

	return "", fmt.Errorf("no update binary for %v/%v, expected %v", goos, goarch, strings.Join(names, " or "))
}

// fileSHA256 returns the SHA-256 hash of the file. It reads the file again
// only when its size or modification time changes.
func (s *Server) fileSHA256(fpath string) (string, error) {
	fi, err := os.Stat(fpath)
	if err != nil {
		return "", err
	}

	s.hashLock.Lock()
	defer s.hashLock.Unlock()

	if h, ok := s.hashes[fpath]; ok && h.size == fi.Size() && h.modTime.Equal(fi.ModTime()) {
		return h.sum, nil
	}

	sum, err := FileSHA256(fpath)
	if err != nil {
		return "", err
	}

	if s.hashes == nil {
		s.hashes = make(map[string]stagedHash)
	}
	s.hashes[fpath] = stagedHash{size: fi.Size(), modTime: fi.ModTime(), sum: sum}

	return sum, nil
}

// FileSHA256 returns the hex-encoded SHA-256 hash of the file.
func FileSHA256(fpath string) (string, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// updateFileFor returns the path of the binary to send to the client, or an
// empty string when the client does not get an update.
func (s *Server) updateFileFor(c *Client) string {
	if !s.AutoUpdate() {
		return ""
	}

	if !c.HasCapability(CapAutoUpdate) {
		if c.Version != version.Revision {
			log.Warn("miniccc on %v (revision %v) does not support auto-update and needs a manual update", c.Hostname, c.Version)
		}

		return ""
	}

	fpath, err := s.updateFilePath(c.OS, c.Arch)
	if err != nil {
		log.Warn("no miniccc auto-update for %v: %v", c.Hostname, err)
		return ""
	}

	sum, err := s.fileSHA256(fpath)
	if err != nil {
		log.Error("no miniccc auto-update for %v: %v", c.Hostname, err)
		return ""
	}

	if sum == c.SHA256 {
		return ""
	}

	return fpath
}

// startUpdate marks the client as updating and sends it the update binary.
// Assumes that clientLock is held.
func (s *Server) startUpdate(c *client, fpath string) {
	c.updating = true
	host := c.Hostname

	log.Info("starting miniccc auto-update for %v with %v", host, fpath)
	if n := len(c.Processes); n > 0 {
		log.Warn("miniccc auto-update for %v kills %v background processes", host, n)
	}

	go func() {
		if err := s.sendUpdateFile(c, fpath); err != nil {
			log.Error("miniccc auto-update for %v: %v", host, err)
			s.endUpdate(c, 0)
			return
		}

		timeout := s.updateTimeout
		if timeout == 0 {
			timeout = UPDATE_TIMEOUT * time.Second
		}

		time.AfterFunc(timeout, func() { s.endUpdate(c, timeout) })
	}()
}

// endUpdate clears the update mark of the client and sends the commands that
// the server held. A non-zero timeout means that the client did not reconnect
// in time. It returns false when the client is not connected or not updating.
func (s *Server) endUpdate(c *client, timeout time.Duration) bool {
	s.clientLock.Lock()
	uuid, host := c.UUID, c.Hostname
	held := c.updating && s.clients[uuid] == c
	c.updating = false
	s.clientLock.Unlock()

	if !held {
		return false
	}

	if timeout > 0 {
		log.Warn("miniccc on %v did not reconnect %v after its update, sending held commands", host, timeout)
	}
	s.sendCommands(uuid)

	return true
}

// sendUpdateFile sends the update binary to the client in chunks.
func (s *Server) sendUpdateFile(c *client, fpath string) error {
	// SendFile reports some errors to the client and then returns nil
	var sendErr string
	send := func(m *Message) error {
		if m.File != nil {
			m.File.Update = true
		}
		if m.Error != "" {
			sendErr = m.Error
		}
		return c.sendMessage(m)
	}

	if err := SendFile(filepath.Dir(fpath), fpath, 0, PART_SIZE, send); err != nil {
		return err
	}
	if sendErr != "" {
		return errors.New(sendErr)
	}

	return nil
}

// holdFor records that the updating client has not received this command.
func (c *Command) holdFor(uuid string) {
	if c.heldFor == nil {
		c.heldFor = make(map[string]bool)
	}

	c.heldFor[uuid] = true
}
