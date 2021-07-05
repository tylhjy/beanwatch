/**
 * @Author tianyang
 * @license: (C) Copyright 2013-2021, NSCC-TJ.AllRightsReserved.
 * @Description: //TODO $
 * @Date: 16:42 2021/6/30
 * @Software: ThrmsTest
 * @Version:
 * @Param $
 * @return $
 **/
package main

import (
	"encoding/base64"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"os"
	"strings"
)

const AGENT = "agent"
const CLIENT = "client"
const OPENWS = "open"
const CLOSEWS = "close"
const RRCREQ = "rrcreq"
const RRCRES = "rrcres"
const TAILRES = "message"

type TailRequestMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type TailMessage struct {
	Timestamp int64  `json:"timestamp"`
	Content   string `json:"message"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	Type      string `json:"type"`
}

type CmdRequestMessage struct {
	Type string `json:"type"`
	Cmd  string `json:"cmd"`
	Host string `json:"host"`
	ID   string `json:"id"`
}

type CmdMessage struct {
	Timestamp int64  `json:"timestamp"`
	Host      string `json:"host"`
	StdOut    string `json:"stdout"`
	StdErr    string `json:"stderr"`
	Cmd       string `json:"cmd"`
	ID        string `json:"id"`
}

type WSClientInfo struct {
	Host string `json:"host"`
	Path string `json:"path"`
	ID   string `json:"id"`
}

type WSRrcInfo struct {
	Host string `json:"host"`
	CMD  string `json:"cmd"`
	ID   string `json:"id"`
}

type WSAgentInfo struct {
	Host string `json:"host"`
}

type WSClientFunc interface {
	Hash() string
}

type WSAgentFunc interface {
	Hash() string
}

func (wsi *WSClientInfo) Hash() string {
	var str string
	str = fmt.Sprintf("%s_%s_%s", CLIENT, wsi.Host, wsi.Path)
	strBytes := []byte(str)
	encoded := base64.StdEncoding.EncodeToString(strBytes)
	return encoded
}

func (wri *WSRrcInfo) Hash() string {
	var str string
	str = fmt.Sprintf("%s_%s_%s_%s", CLIENT, wri.Host, wri.CMD, wri.ID)
	strBytes := []byte(str)
	encoded := base64.StdEncoding.EncodeToString(strBytes)
	return encoded
}

func (wai *WSAgentInfo) Hash() string {
	var str string
	str = fmt.Sprintf("%s_%s", AGENT, wai.Host)
	strBytes := []byte(str)
	encoded := base64.StdEncoding.EncodeToString(strBytes)
	return encoded
}

// Get Tail Agent from host
func GetBeanAgent(host string) (*websocket.Conn) {
	wai := &WSAgentInfo{
		Host: host,
	}
	if _, ok := agents[wai.Hash()]; !ok {
		return nil
	}
	return agents[wai.Hash()]
}

// Get Tail Client from Tail Message
func GetTailClient(message *TailMessage) (string, map[string]*websocket.Conn) {
	wsi := WSClientInfo{
		Host: message.Host,
		Path: message.Path,
		ID:   "",
	}
	hash := wsi.Hash()
	return hash, clients[hash]
}

// Get Tail Client from Tail Message
func GetRrcClient(message *CmdMessage) (string, *websocket.Conn) {
	wri := WSRrcInfo{
		Host: message.Host,
		CMD:  message.Cmd,
		ID:   message.ID,
	}
	hash := wri.Hash()
	return hash, rrcs[hash]
}

// Response broadcast channel
var responseBroadcast = make(chan TailMessage)
var rrcBroadcast = make(chan CmdMessage)

// Configure the upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// agents saved all agent web sockets clients save all client web sockets
var agents = make(map[string]*websocket.Conn)
var clients = make(map[string](map[string]*websocket.Conn))
var rrcs = make(map[string]*websocket.Conn)

// Get remote addr
func GetHost(remoteAddr string) string {
	if strings.Index(remoteAddr, ":") >= 0 {
		return strings.Split(remoteAddr, ":")[0]
	} else {
		return remoteAddr
	}
}

//Bean agent: this websocket is mainly receiving message from Bean agent for tail and other command
func BeanAgent(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	remoteHost := GetHost(r.RemoteAddr)
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	ws.SetReadLimit(1024 * 1024 * 8)
	// Make sure we close the connection when the function returns
	defer ws.Close()
	// register new agent
	agent := &WSAgentInfo{
		Host: remoteHost,
	}

	agentHash := agent.Hash()
	agents[agentHash] = ws
	fmt.Println("Register agent", remoteHost)
	for {
		var tailMessage TailMessage
		var cmdMessage CmdMessage
		// Read in a new message as JSON and map it to a Message object
		var message map[string]interface{}
		err := ws.ReadJSON(&message)
		//fmt.Println("from tail agent message is:", message)
		if err != nil {
			log.Printf("read tail agent message failed: %s", err)
			delete(agents, agentHash)
			break
		}
		// Send the newly received message to the broadcast channel
		if message["type"].(string) == TAILRES {
			tailMessage = TailMessage{
				Timestamp: int64(message["timestamp"].(float64)),
				Content:   message["message"].(string),
				Host:      message["host"].(string),
				Path:      message["path"].(string),
				Type:      message["type"].(string),
			}
			responseBroadcast <- tailMessage
		} else if message["type"].(string) == RRCRES {
			cmdMessage = CmdMessage{
				Timestamp: int64(message["timestamp"].(float64)),
				StdOut:    message["stdout"].(string),
				StdErr:    message["stderr"].(string),
				Cmd:       message["cmd"].(string),
				Host:      message["host"].(string),
				ID:        message["id"].(string),
			}
			rrcBroadcast <- cmdMessage
		}
	}
}

//Tail request: this websocket is mainly sending agent's message to tail request client
func BeanTailClient(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	ws.SetReadLimit(1024 * 1024 * 8)
	// Make sure we close the connection when the function returns
	defer ws.Close()
	// Get host and tail file path
	vars := mux.Vars(r)
	host := vars["host"]
	path := r.URL.Query().Get("path")
	// fmt.Println("r.RemoteAddr==",r.RemoteAddr,path)
	id := r.RemoteAddr
	// Register new tail client
	client := &WSClientInfo{
		Host: host,
		Path: path,
		ID:   id,
	}
	clientHash := client.Hash()
	if clients[clientHash] == nil {
		fmt.Println("tail new file:", path, "on host:", host)
		clients[clientHash] = make(map[string]*websocket.Conn)
	}
	fmt.Println("tail file:", path, "on host:", host, "client:", r.RemoteAddr, "on line!")
	clients[clientHash][id] = ws
	agent := GetBeanAgent(host)
	if agent == nil {
		_ = ws.WriteJSON(map[string]string{
			"type":    "error",
			"message": "host agent is not on line",
		})
		return
	}
	// tell agent to open new tail go routine
	err = agent.WriteJSON(TailRequestMessage{
		Type: OPENWS,
		Path: path,
	})
	// read request message
	for {
		message := make(map[string]interface{})
		err := ws.ReadJSON(&message)
		// fmt.Println("from tail client message is:", message)
		if err != nil {
			log.Printf("request ws recieved : %s", err)
			// The client is disconnected, and at this time, it is judged whether there is a request for host+path, and if not, a message is sent to the agent on the responding host to close the go routine
			delete(clients[clientHash], id)
			if len(clients[clientHash]) == 0 {
				SendCloseConnToAgent(host, path)
			}
			break
		}
	}
}

//Run remote command request: this websocket is mainly sending agent's command to run on itself
func BeanCmdClient(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	ws.SetReadLimit(1024 * 1024 * 8)
	// Make sure we close the connection when the function returns
	defer ws.Close()
	// Get host and cmd
	vars := mux.Vars(r)
	host := vars["host"]
	cmdEncode := r.URL.Query().Get("cmd")
	decoded, _ := base64.StdEncoding.DecodeString(cmdEncode)
	cmd := string(decoded)
	id := r.RemoteAddr
	// Register new cmd client
	rrc := &WSRrcInfo{
		Host: host,
		CMD:  cmd,
		ID:   id,
	}
	rrcHash := rrc.Hash()
	rrcs[rrcHash] = ws
	agent := GetBeanAgent(host)
	if agent == nil {
		_ = ws.WriteJSON(map[string]string{
			"type":    "error",
			"message": "host agent is not on line",
		})
		return
	}
	// tell agent to run command
	err = agent.WriteJSON(CmdRequestMessage{
		Type: RRCREQ,
		Cmd:  cmd,
		Host: host,
		ID:   id,
	})
	// wait for agent message
	var wt = make(chan int)
	for {
		<- wt
	}
}

// tell agent to close
func SendCloseConnToAgent(host string, path string) {
	a := GetBeanAgent(host)
	err := a.WriteJSON(TailRequestMessage{
		Type: CLOSEWS,
		Path: path,
	})
	fmt.Println(err)
}

var Port string = "8001"

func main() {
	if len(os.Args) > 1 {
		Port = os.Args[1]
	}
	r := mux.NewRouter()
	// Configure websocket route
	r.HandleFunc("/v1/tail/file/client/{host}/ws", BeanTailClient)
	//run/cmd/client/%s/ws?cmd=%s
	r.HandleFunc("/v1/run/cmd/client/{host}/ws", BeanCmdClient)
	r.HandleFunc("/v1/tail/file/agent/ws", BeanAgent)
	// Start listening for incoming messages
	go HandleTailMessages()
	go HandleRRCMessages()
	// Start the server on localhost port 8000 and log any errors
	log.Println(fmt.Sprintf("http server started on :%s", Port))
	err := http.ListenAndServe(fmt.Sprintf(":%s", Port), r)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// Handle Tail Messages
func HandleTailMessages() {
	for {
		// Grab the next message from the broadcast channel
		message := <-responseBroadcast
		_, cs := GetTailClient(&message)
		for _, c := range cs {
			err := c.WriteJSON(message)
			if err != nil {
				log.Printf("error: %v", err)
				_ = c.Close()
			}
		}
	}
}

// Handle remote command messages
func HandleRRCMessages() {
	for {
		// Grab the next message from the broadcast channel
		result := <-rrcBroadcast
		hash, c := GetRrcClient(&result)
		err := c.WriteJSON(map[string]interface{}{
			"timestamp": result.Timestamp,
			"stdout":    result.StdOut,
			"stderr":    result.StdErr,
			"cmd":       result.Cmd,
			"type":      RRCRES,
		})
		if err != nil {
			log.Printf("error: %v", err)
		}
		// delete command client websocket
		// _ = c.Close()
		delete(rrcs, hash)
	}
}
