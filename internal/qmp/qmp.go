// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// A qemu qmp wrapper. qmp will connect to qmp unix domain sockets associated with running instances of qemu.
package qmp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var ERR_READY = errors.New("qmp is not ready")

type Conn struct {
	socket       string // path to unix domain socket to connect to
	conn         net.Conn
	dec          *json.Decoder
	enc          *json.Encoder
	messageSync  chan map[string]interface{}
	messageAsync chan map[string]interface{}
	ready        bool
}

// return an asynchronous message, blocking until one shows up
func (q *Conn) Message() map[string]interface{} {
	return <-q.messageAsync
}

// Connect to a qmp socket.
func Dial(s string) (Conn, error) {
	var q Conn
	err := q.connect(s)
	return q, err
}

func (q *Conn) connect(s string) error {
	log.Debug("qmp connect: %v", s)

	q.socket = s
	conn, err := net.Dial("unix", q.socket)
	if err != nil {
		return err
	}
	q.conn = conn
	q.dec = json.NewDecoder(q.conn)
	q.enc = json.NewEncoder(q.conn)
	q.messageSync = make(chan map[string]interface{}, 1024)
	q.messageAsync = make(chan map[string]interface{}, 1024)

	// upon connecting we should get the qmp version etc.
	v, err := q.read()
	if err != nil {
		conn.Close()
		return err
	}

	v = map[string]interface{}{
		"execute": "qmp_capabilities",
	}
	err = q.enc.Encode(&v)
	if err != nil {
		conn.Close()
		return err
	}

	v, err = q.read()
	if err != nil {
		conn.Close()
		return err
	}
	if !success(v) {
		conn.Close()
		return errors.New("failed success")
	}

	go q.reader()

	q.ready = true

	return nil
}

func success(v map[string]interface{}) bool {
	for k, e := range v {
		if k != "return" {
			return false
		}
		switch t := e.(type) {
		case map[string]interface{}:
			if len(t) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (q *Conn) read() (map[string]interface{}, error) {
	var v map[string]interface{}
	err := q.dec.Decode(&v)
	if err != nil {
		return nil, err
	}
	log.Debug("qmp read: %#v", v)
	return v, nil
}

func (q *Conn) write(v map[string]interface{}) error {
	log.Debug("qmp write: %#v", v)
	if !q.ready {
		return ERR_READY
	}
	err := q.enc.Encode(&v)
	return err
}

func (q *Conn) Raw(input string) (string, error) {
	log.Debug("qmp write: %v", input)
	if !q.ready {
		return "", ERR_READY
	}
	_, err := q.conn.Write([]byte(input))
	if err != nil {
		return "", err
	}
	v := <-q.messageSync
	status := v["return"]
	if status == nil {
		return "", errors.New("received nil status")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err = enc.Encode(&v)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (q *Conn) Status() (map[string]interface{}, error) {
	if !q.ready {
		return nil, ERR_READY
	}
	s := map[string]interface{}{
		"execute": "query-status",
	}
	err := q.write(s)
	if err != nil {
		return nil, err
	}
	v := <-q.messageSync
	status := v["return"]
	if status == nil {
		return nil, errors.New("received nil status")
	}
	return status.(map[string]interface{}), nil
}

func (q *Conn) Start() error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "cont",
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("could not start VM")
	}
	return nil
}

func (q *Conn) Stop() error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "stop",
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("could not stop VM")
	}
	return nil
}

func (q *Conn) BlockdevEject(device string, force bool) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "eject",
		"arguments": map[string]interface{}{
			"device": device,
			"force":  force,
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("eject")
	}
	return nil
}

func (q *Conn) BlockdevChange(device, path string) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "change",
		"arguments": map[string]interface{}{
			"device": device,
			"target": path,
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("change")
	}
	return nil
}

func (q *Conn) Pmemsave(path string, size uint64) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "pmemsave",
		"arguments": map[string]interface{}{
			"val":      0,
			"size":     size,
			"filename": path,
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("pmemsave")
	}
	return nil
}

func (q *Conn) BlockdevSnapshot(path, device string) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "blockdev-snapshot",
		"arguments": map[string]interface{}{
			"device":        device,
			"snapshot-file": path,
			"format":        "raw",
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("blockdev_snapshot")
	}
	return nil
}

func (q *Conn) Screendump(path string) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "screendump",
		"arguments": map[string]interface{}{
			"filename": fmt.Sprintf("%v", path),
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("screendump")
	}
	return nil
}

func (q *Conn) SaveDisk(path, device string) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "drive-backup",
		"arguments": map[string]interface{}{
			"device": device,
			"sync":   "top",
			"target": path,
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("error in qmp SaveDisk")
	}
	return nil
}

func (q *Conn) MigrateDisk(path string) error {
	if !q.ready {
		return ERR_READY
	}
	s := map[string]interface{}{
		"execute": "migrate",
		"arguments": map[string]interface{}{
			"uri": fmt.Sprintf("exec:cat > %v", path),
		},
	}
	err := q.write(s)
	if err != nil {
		return err
	}
	v := <-q.messageSync
	if !success(v) {
		return errors.New("migrate")
	}
	return nil
}

func (q *Conn) QueryMigrate() (map[string]interface{}, error) {
	if !q.ready {
		return nil, ERR_READY
	}
	s := map[string]interface{}{
		"execute": "query-migrate",
	}
	err := q.write(s)
	if err != nil {
		return nil, err
	}
	v := <-q.messageSync

	status := v["return"]
	if status == nil {
		return nil, errors.New("received nil status")
	}
	return status.(map[string]interface{}), nil
}

func (q *Conn) QueryBlock() ([]interface{}, error) {
	if !q.ready {
		return nil, ERR_READY
	}
	s := map[string]interface{}{
		"execute": "query-block",
	}
	err := q.write(s)
	if err != nil {
		return nil, err
	}
	v := <-q.messageSync
	status := v["return"]
	if status == nil {
		return nil, errors.New("received nil status")
	}
	return status.([]interface{}), nil
}

func (q *Conn) QueryBlockJobs() ([]interface{}, error) {
	if !q.ready {
		return nil, ERR_READY
	}
	s := map[string]interface{}{
		"execute": "query-block-jobs",
	}
	err := q.write(s)
	if err != nil {
		return nil, err
	}
	v := <-q.messageSync

	status := v["return"]
	if status == nil {
		return nil, errors.New("received nil status")
	}
	return status.([]interface{}), nil
}

func (q *Conn) HumanMonitorCommand(command string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	s := map[string]interface{}{
		"execute": "human-monitor-command",
		"arguments": map[string]interface{}{
			"command-line": command,
		},
	}
	err := q.write(s)
	if err != nil {
		return "", err
	}
	v := <-q.messageSync
	response := v["return"]
	if response == nil {
		return "", errors.New("received nil response")
	}
	return response.(string), nil
}

func (q *Conn) DriveAdd(id, file string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	arg := fmt.Sprintf("drive_add 0 id=%v,if=none,file=%v", id, file)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) USBDeviceAdd(id, bus, serial string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	arg := fmt.Sprintf("device_add usb-storage,id=%v,drive=%v,bus=%v", id, id, bus)
	if serial != "" {
		arg = fmt.Sprintf("device_add usb-storage,id=%v,drive=%v,bus=%v,serial=%v", id, id, bus, serial)
	}
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) CCIDAdd() (string, error) {
	if !q.ready {
		return "", ERR_READY
	}

	arg := fmt.Sprintf("device_add usb-ccid")

	log.Debugln("sending qmp command: ", arg)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err 
}

func (q *Conn) SmartcardAdd(id, smartcard_path string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	var arg string 


	default_options := fmt.Sprintf("backend=certificates,cert1=id-cert,cert2=signing-cert,cert3=encryption-cert")

	if smartcard_path != "" {
		arg = fmt.Sprintf("device_add ccid-card-emulated,id=%v,%v,db=sql:%v", id, default_options, smartcard_path)
	} else {
		arg = fmt.Sprintf("device_add ccid-card-emulated,id=%v", id)
	}
	log.Debugln("sending qmp command: ", arg)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) SmartcardRemove(id string) (string, error) {
	resp, err := q.USBDeviceDel(id)
	return resp, err 
}


func (q *Conn) NetDevAdd(devType, id, ifname string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	arg := fmt.Sprintf("netdev_add type=%v,id=%v,ifname=%v,script=no,downscript=no", devType, id, ifname)
	log.Debugln("sending qmp command: ", arg)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) NicAdd(id, netdevID, bus, driver, mac string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	//arg := fmt.Sprintf("device_add id=%v,netdev=%v,bus=%v,addr=%v,driver=%v,mac=%v", id, netdevID, bus, addr, driver, mac)
	arg := fmt.Sprintf("device_add id=%v,netdev=%v,bus=%v,driver=%v,mac=%v", id, netdevID, bus, driver, mac)
	log.Debugln("sending qmp command: ", arg)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) USBDeviceDel(id string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	arg := fmt.Sprintf("device_del %v", id)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) DriveDel(id string) (string, error) {
	if !q.ready {
		return "", ERR_READY
	}
	arg := fmt.Sprintf("drive_del %v", id)
	resp, err := q.HumanMonitorCommand(arg)
	return resp, err
}

func (q *Conn) reader() {
	for {
		v, err := q.read()
		if err != nil {
			close(q.messageAsync)
			close(q.messageSync)
			break
		}
		// split asynchronous and synchronous events.
		if v["event"] != nil {
			q.messageAsync <- v
		} else {
			q.messageSync <- v
		}
	}
}
