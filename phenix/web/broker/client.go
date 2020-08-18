package broker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"phenix/api/experiment"
	"phenix/api/vm"
	"phenix/types"
	"phenix/web/rbac"
	"phenix/web/util"

	log "github.com/activeshadow/libminimega/minilog"
	"github.com/gorilla/websocket"
)

type vmScope struct {
	exp  string
	name string
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 5 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 512
)

var (
	newline  = []byte{'\n'}
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type Client struct {
	sync.RWMutex

	role rbac.Role
	conn *websocket.Conn

	publish chan interface{}
	done    chan struct{}
	once    sync.Once

	// Track the VMs this client currently has in view, if any, so we know
	// what screenshots need to periodically be pushed to the client over
	// the WebSocket connection.
	vms []vmScope
}

func NewClient(role rbac.Role, conn *websocket.Conn) *Client {
	log.Debug("[gophenix] new WS client created")

	return &Client{
		role:    role,
		conn:    conn,
		publish: make(chan interface{}, 256),
		done:    make(chan struct{}),
	}
}

func (this *Client) Go() {
	register <- this

	go this.write()
	go this.read()
	go this.screenshots()
}

func (this *Client) Stop() {
	this.once.Do(this.stop)
}

func (this *Client) stop() {
	unregister <- this
	close(this.done)
	this.conn.Close()

	log.Debug("[gophenix] WS client destroyed")
}

func (this *Client) read() {
	defer this.Stop()

	this.conn.SetReadLimit(maxMsgSize)
	this.conn.SetReadDeadline(time.Now().Add(pongWait))

	ponger := func(string) error {
		this.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	}

	this.conn.SetPongHandler(ponger)

	for {
		_, msg, err := this.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Debug("reading from WebSocket client: %v", err)
			}

			break
		}

		var req Request
		if err := json.Unmarshal(msg, &req); err != nil {
			log.Error("cannot unmarshal request JSON: %v", err)
			continue
		}

		switch req.Resource.Type {
		case "experiment/vms":
		default:
			log.Error("unexpected WebSocket request resource type: %s", req.Resource.Type)
			continue
		}

		switch req.Resource.Action {
		case "list":
		default:
			log.Error("unexpected WebSocket request resource action: %s", req.Resource.Action)
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			log.Error("cannot unmarshal WebSocket request payload JSON: %v", err)
			continue
		}

		if !this.role.Allowed("vms", "list") {
			log.Warn("client access to vms/list forbidden")
			continue
		}

		expName := req.Resource.Name

		exp, err := experiment.Get(expName)
		if err != nil {
			log.Error("getting experiment %s for WebSocket client: %v", expName, err)
			continue
		}

		vms, err := vm.List(expName)
		if err != nil {
			// TODO
		}

		// If `filter` was not provided client-side, `regex` will be an
		// empty string and the call to `regexp.MatchString` below will
		// always be true.
		regex, _ := payload["filter"].(string)

		allowed := types.VMs{}

		for _, vm := range vms {
			// If there's an error, `matched` will be false.
			if matched, _ := regexp.MatchString(regex, vm.Name); !matched {
				continue
			}

			if this.role.Allowed("vms", "list", fmt.Sprintf("%s_%s", expName, vm.Name)) {
				if vm.Running {
					screenshot, err := util.GetScreenshot(expName, vm.Name, "200")
					if err != nil {
						log.Error("getting screenshot for WebSocket client: %v", err)
					} else {
						vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
					}
				}

				allowed = append(allowed, vm)
			}
		}

		var (
			sort = payload["sort_column"].(string)
			asc  = payload["sort_asc"].(bool)
			page = int(payload["page_number"].(float64))
			size = int(payload["page_size"].(float64))
		)

		// Reusing `payload` variable here for response.
		payload = map[string]interface{}{"total": len(allowed)}

		if sort != "" {
			allowed.SortBy(sort, asc)
		}

		if page != 0 && size != 0 {
			allowed = allowed.Paginate(page, size)
		}

		this.Lock()

		this.vms = nil

		for _, v := range allowed {
			this.vms = append(this.vms, vmScope{exp: expName, name: v.Name})
		}

		this.Unlock()

		payload["vms"] = allowed

		marshalled, err := json.Marshal(payload)
		if err != nil {
			log.Error("marshaling experiment %s VMs for WebSocket client: %v", exp, err)
			continue
		}

		this.publish <- Publish{
			Resource: NewResource("experiment/vms", expName, "list"),
			Result:   marshalled,
		}
	}
}

func (this *Client) write() {
	defer this.Stop()

	ticker := time.NewTicker(pingPeriod)

	defer ticker.Stop()

	for {
		select {
		case <-this.done:
			this.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case msg := <-this.publish:
			this.conn.SetWriteDeadline(time.Now().Add(writeWait))

			w, err := this.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			b, err := json.Marshal(msg)
			if err != nil {
				log.Error("marshaling message to be published: %v", err)
				continue
			}

			w.Write(b)

			for i := 0; i < len(this.publish); i++ {
				w.Write(newline)

				msg := <-this.publish

				b, err := json.Marshal(msg)
				if err != nil {
					log.Error("marshaling message to be published: %v", err)
					continue
				}

				w.Write(b)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			this.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := this.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (this *Client) screenshots() {
	defer this.Stop()

	ticker := time.NewTicker(5 * time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-this.done:
			return
		case <-ticker.C:
			this.RLock()

			for _, v := range this.vms {
				screenshot, err := util.GetScreenshot(v.exp, v.name, "200")
				if err != nil {
					log.Error("getting screenshot for WebSocket client: %v", err)
				} else {
					encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
					marshalled, err := json.Marshal(util.WithRoot("screenshot", encoded))
					if err != nil {
						log.Error("marshaling VM %s screenshot for WebSocket client: %v", v, err)
						continue
					}

					this.publish <- Publish{
						Resource: NewResource("experiment/vm/screenshot", fmt.Sprintf("%s/%s", v.exp, v.name), "update"),
						Result:   marshalled,
					}
				}
			}

			this.RUnlock()
		}
	}
}

func ServeWS(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(*http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("upgrading connection to WebSocket: %v", err)
		return
	}

	role := r.Context().Value("role").(rbac.Role)

	NewClient(role, conn).Go()
}
