// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	log "minilog"
	"net"
	"os"
	"path/filepath"
	"ron"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"vnc"
)

type ServerInfo struct {
	Width  uint16
	Height uint16
}

type RKVMConfig struct {
	Vnc_host string
	Vnc_port int
}

type RKvmVM struct {
	*BaseVM     // embed
	RKVMConfig  // embed
	vncShim     net.Listener
	vncShimPort int
}

// Ensure that RKVM implements the VM interface
var _ VM = (*RKvmVM)(nil)
var killVnc = make(chan bool, 1)

// Copy makes a copy and returns reference to the new struct.
func (old RKVMConfig) Copy() RKVMConfig {
	res := old
	return res
}

func (vm *RKvmVM) ProcStats() (map[int]*ProcStats, error) {
	p, err := GetProcStats(vm.Pid)
	if err != nil {
		return nil, err
	}
	return map[int]*ProcStats{vm.Pid: p}, nil
}

func NewRKVM(name, namespace string, config VMConfig) (*RKvmVM, error) {
	vm := new(RKvmVM)

	vm.BaseVM = NewBaseVM(name, namespace, config)
	vm.Type = RKVM

	vm.RKVMConfig = config.RKVMConfig.Copy() // deep-copy configured fields

	return vm, nil
}

func (vm *RKvmVM) Copy() VM {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm2 := new(RKvmVM)

	// Make shallow copies of all fields
	*vm2 = *vm

	// Make deep copies
	vm2.BaseVM = vm.BaseVM.copy()
	vm2.RKVMConfig = vm.RKVMConfig.Copy()

	return vm2
}

// Launch a new RKVM VM
func (vm *RKvmVM) Launch() error {
	defer vm.lock.Unlock()
	return vm.launch()
}

// Flush cleans up all resources allocated to the VM
func (vm *RKvmVM) Flush() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()
	return vm.BaseVM.Flush()
}

func (vm *RKvmVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func (vm *RKvmVM) Start() (err error) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&VM_RUNNING != 0 {
		return nil
	}

	if vm.State == VM_QUIT || vm.State == VM_ERROR {
		log.Info("relaunching VM: %v", vm.ID)
		// Create a new channel since we closed the other one to indicate that
		// the VM should quit.
		vm.kill = make(chan bool)

		// Launch handles setting the VM to error state
		if err := vm.launch(); err != nil {
			return err
		}
	}

	log.Info("starting VM: %v", vm.ID)
	//fmt.Println("starting id " + strconv.Itoa(vm.ID))
	vm.setState(VM_RUNNING)

	return nil
}

func (vm *RKvmVM) Stop() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.Name == "vince" {
		return errors.New("vince is stoppable, however one must posses a rare whiskey")
	}

	if vm.State != VM_RUNNING {
		return vmNotRunning(strconv.Itoa(vm.ID))
	}

	log.Info("stopping VM: %v", vm.ID)

	vm.setState(VM_PAUSED)

	return nil
}

func (vm *RKvmVM) String() string {
	return fmt.Sprintf("%s:%d:rkvm", vm.Vnc_host, vm.ID)
}

func (vm *RKvmVM) Info(field string) (string, error) {
	// If the field is handled by BaseVM, return it
	if v, err := vm.BaseVM.Info(field); err == nil {
		return v, nil
	}

	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch field {
	case "vnc_port":
		return strconv.Itoa(vm.vncShimPort), nil
	case "host":
		return vm.Vnc_host, nil
	}

	return vm.RKVMConfig.Info(field)
}

// Tests whether rkvm types conflict with container and kvm types
func (vm *RKvmVM) Conflicts(vm2 VM) error {
	switch vm2 := vm2.(type) {
	case *RKvmVM:
		return vm.ConflictsRKVM(vm2)
	case *KvmVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	case *ContainerVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)

	}

	return errors.New("unknown VM type")
}

// ConflictsRKVM tests whether vm and vm2 share a vnc host, returns an
// error if so. Also checks whether the BaseVMs conflict.
func (vm *RKvmVM) ConflictsRKVM(vm2 *RKvmVM) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.Vnc_host == vm2.Vnc_host {
		return fmt.Errorf("duplicate rkvm %v: %v", vm.Name, vm2.Name)
	}
	return vm.BaseVM.conflicts(vm2.BaseVM)
}

func (vm *RKVMConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "RKVM configuration:")
	fmt.Fprintf(w, "Vnc Host:\t%v\n", vm.Vnc_host)
	fmt.Fprintf(w, "Vnc Port:\t%v\n", vm.Vnc_port)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (vm *RKvmVM) Screenshot(size int) ([]byte, error) {
	if vm.State != VM_RUNNING {
		return nil, nil
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_screenshot_rkvm_%v", vm.ID))
	ppmFile, err := ioutil.ReadFile(tmp)
	if err != nil {
		log.Error("Unable to read " + tmp)
		return nil, err
	}
	pngResult, err := ppmToPng(ppmFile, size)
	if err != nil {
		log.Error("Error converting screenshot")
		return nil, err
	}
	return pngResult, nil

}

func (vm *RKvmVM) Connect(cc *ron.Server, reconnect bool) error {
	if !vm.Backchannel {
		return nil
	}

	if !reconnect {
		cc.RegisterVM(vm)
	}
	return nil
}

func (vm *RKvmVM) Disconnect(cc *ron.Server) error {
	if !vm.Backchannel {
		return nil
	}
	cc.UnregisterVM(vm)
	return nil
}

// launch is the low-level launch function for KVM VMs. The caller should hold
// the VM's lock.
func (vm *RKvmVM) launch() error {
	log.Info("launching vm: %v", strconv.Itoa(vm.ID))
	// If this is the first time launching the VM, do the final configuration
	// check and create directories for it.
	if vm.State == VM_BUILDING {
		// create a directory for the VM at the instance path
		if err := os.MkdirAll(vm.instancePath, os.FileMode(0700)); err != nil {
			return vm.setErrorf("unable to create VM dir: %v", err)
		}
		// Create a snapshot of each disk image
	}
	mustWrite(vm.path("name"), vm.Name)

	// create and add taps if we are associated with any networks
	vmConfig := VMConfig{BaseConfig: vm.BaseConfig, RKVMConfig: vm.RKVMConfig}

	// Create goroutine to wait to kill the VM
	go func() {
		defer vm.cond.Signal()
		select {
		case <-vm.kill:
			log.Info("Killing VM %v", vm.ID)
			killVnc <- true
			vm.setState(VM_QUIT)
			return
		}
	}()

	err := vm.connectVNC(vmConfig)
	return err
}

func (vm *RKvmVM) connectVNC(config VMConfig) error {
	// should never create...
	ns := GetOrCreateNamespace(vm.Namespace)
	connectionString := config.RKVMConfig.Vnc_host + ":" + strconv.Itoa(config.RKVMConfig.Vnc_port)

	shim, shimErr := net.Listen("tcp", "")
	if shimErr != nil {
		log.Error("Shim listen error")
		return shimErr
	}
	vm.vncShim = shim
	vm.vncShimPort = shim.Addr().(*net.TCPAddr).Port
	log.Info("Connecting to Server at :" + strconv.Itoa(vm.vncShimPort))
	var vncRx_c = make(chan interface{}, 1)
	var vncTx_c = make(chan interface{}, 1)
	var serverIntInfo_c = make(chan ServerInfo, 1)
	var killFbRes_c = make(chan bool, 1)
	var killFbReq_c = make(chan bool, 1)
	var killVncRx_c = make(chan bool, 1)
	var killShim_c = make(chan bool, 1)
	var resetCon_c = make(chan bool, 1)
	var conError_c = make(chan error, 1)

	go func() {
		defer shim.Close()
		for {
			remote, err := shim.Accept()
			if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
				return
			} else if err != nil {
				log.Errorln(err)
				return
			}
			log.Debug("SHIM")
			log.Info("vnc shim connect: %v -> %v", remote.RemoteAddr(), vm.Name)
			go func() {
				defer remote.Close()
				// Dial domain socket
				local, err := net.Dial("tcp", connectionString)

				if err != nil {
					log.Error("unable to dial vm vnc: %v", err)
					return
				}
				defer local.Close()
				// copy local -> remote
				go io.Copy(remote, local)
				// Reads will implicitly copy from remote -> local
				tee := io.TeeReader(remote, local)
				for {
					msg, err := vnc.ReadClientMessage(tee)
					if err == nil {
						ns.Recorder.Route(vm.GetName(), msg)
						continue
					}
					// shim is no longer connected
					if err == io.EOF || strings.Contains(err.Error(), "broken pipe") {
						log.Info("vnc shim quit: %v", vm.Name)
						break
					}
					// ignore these
					if strings.Contains(err.Error(), "unknown client-to-server message") {
						continue
					}
					//unknown error
					log.Warnln(err)
				}

			}()
		}
	}()

	go func(vm *RKvmVM) {
		var sin ServerInfo
		var con *vnc.Conn
		var conErr error
		var reset bool = true
		var errorCount int = 0
		var conErrorCount int = 0
		var connected bool = false
		var ID = vm.ID

		for {
			select {
			case reset = <-resetCon_c:
				if connected {
					killFbReq_c <- true
					killFbRes_c <- true
					killVncRx_c <- true
					time.Sleep(2 * time.Second)
					con.Close()

				}
				break
			case <-killVnc:
				killFbReq_c <- true
				killFbRes_c <- true
				killVncRx_c <- true
				killShim_c <- true
				_, _ = vnc.Dial("127.0.0.1" + ":" + strconv.Itoa(vm.vncShimPort))
				time.Sleep(2 * time.Second)
				con.Close()

				return
			case tx := <-vncTx_c:
				//fmt.Println("TX Signal Comm")
				switch tx.(type) {
				case *vnc.FramebufferUpdateRequest:
					//fmt.Println("TX FB Request")
					sender := tx.(*vnc.FramebufferUpdateRequest)
					if err := sender.Write(con); err != nil {
						log.Error("unable to request framebuffer update")
						errorCount++
					}
				default:
					log.Error("unimplemented send type in vncTx_c channel")
				}
				break
			default:
				//fmt.Println("No signals communicating")
				//break
			} //end select for channel communication
			if errorCount > 3 {
				errorCount = 0
				resetCon_c <- true
			}
			if reset {
				log.Info("RESET VNC Connection")
				reset = false
				con, conErr = vnc.Dial(connectionString)
				if conErr != nil {
					log.Error("unable to dial vm vnc %v: %v", connectionString, conErr)
					conErrorCount++
					connected = false
					resetCon_c <- true
					time.Sleep(5 * time.Second)
					log.Error("RESETTING")
					continue
				}
				if conErrorCount > 12 {
					vm.setErrorf("unable to connect to vnc after 1 minute %v ", conErr)
					log.Error("Exit connect loop")
					killVnc <- true
					continue
				}

				height, width := con.GetDesktopSize()
				sin.Height = height
				sin.Width = width
				serverIntInfo_c <- sin // give information to other routines
				log.Debug("Size %v x %v", width, height)
				connected = true
				//Frame Buffer Request routine
				go func() {
					t0 := time.Now()
					for {
						select {
						case <-killFbReq_c:
							return
						default:
							//break
						}
						t1 := time.Now()
						if t1.Sub(t0).Seconds() > 1 {
							//fmt.Println("Attemping to Request Frame Buffer Update")
							req := &vnc.FramebufferUpdateRequest{}
							req.Width = width - 1
							req.Height = height - 1
							vncTx_c <- req
							t0 = time.Now()
						}
					}
				}()

				//Frame Buffer Response
				go func(ID int) {
					tmp := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_screenshot_rkvm_%v", ID))
					var serverInfo ServerInfo
					serverInfo = <-serverIntInfo_c
					masterFrame := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{int(serverInfo.Width), int(serverInfo.Height)}})
					for {
						//check if we need to kill
						select {
						case <-killFbRes_c:
							log.Debug("RF killed")
							os.Remove(tmp)
							return
						default:
							break
						}
						//fmt.Println("Attempting to read from vnc rx channel")
						msg := <-vncRx_c
						//fmt.Println("vnc rx channel read success")
						switch msg.(type) {
						case *vnc.FramebufferUpdate:
							fb := msg.(*vnc.FramebufferUpdate)
							for _, rect := range fb.Rectangles {
								// ignore non-image
								if rect.RGBA == nil {
									continue
								}
								drawPoint := image.Point{int(rect.X), int(rect.Y)}
								draw.Draw(masterFrame, image.Rect(int(rect.X), int(rect.Y), int(rect.X+rect.Width), int(rect.Height+rect.Y)), rect.RGBA, drawPoint, draw.Src)
							} // end for rectangles
							out, err := os.Create(tmp)
							if err != nil {
								log.Error("Error creating screenshot file %v", err)
							} else {
								err = png.Encode(out, masterFrame)
								out.Close()
								if err != nil {
									log.Error("Error writing out screenshot %v", err)
								}
							}
						} // end message type switch
					} // end for loop
				}(ID) //end Read for Framebuffer Update

				go func(con *vnc.Conn) {
					for {
						select {
						case <-killVncRx_c:
							return
						default:
						}
						//fmt.Println("Attempt Vnc Read")
						msg, err := con.ReadMessage()
						//fmt.Println("VNC Message Read")
						if err == nil {
							ns.Recorder.Route(vm.GetName(), msg)
							vncRx_c <- msg
						} else if strings.Contains(err.Error(), "unknown client-to-server message") {
							//fmt.Println("Unknown message %v",msg)
							log.Error("Unknown message from vnc server")
						} else {
							// unknown error
							log.Warnln(err)
						}
					}
				}(con) //end go func
			} //end if reset
		} //end for loop
	}(vm)
	//error catch
	select {
	case er := <-conError_c:
		return er
	default:
		return nil
	}

}
