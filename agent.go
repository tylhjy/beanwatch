/**
 * @Author tianyang
 * @license: (C) Copyright 2013-2021, NSCC-TJ.AllRightsReserved.
 * @Description: //TODO $
 * @Date: 16:08 2021/6/30
 * @Software: ThrmsTest
 * @Version:
 * @Param $
 * @return $
 **/
package main

import (
	//"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hpcloud/tail"
	"golang.org/x/net/websocket"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	//"os/exec"
	"strings"
	//"syscall"
	"time"
	execute "github.com/alexellis/go-execute/pkg/v1"
)

type Conf struct {
	HostIP string `yaml:"host_ip"`
	TransitServer string `yaml:"transit_server"`
}

// tails saved all tail file info; tailMessage is tail message channel
var tails = make(map[string]bool)
var tailMessage = make(chan []byte)

// Get Config
func GetAgentConf(configFile string) (*Conf,error) {
	var c Conf
	yamlFile, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil,err
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return nil,err
	}
	return &c,nil
}

// TailFile function is a go routine function for tail new file
func TailFile(path string) {
	seek := &tail.SeekInfo{Offset:0,Whence:2}
	tailPtr, err := tail.TailFile(path, tail.Config{
		Follow: true, Poll: true, MustExist: false, ReOpen:true, Location:seek,
	})
	if err != nil {
		fmt.Println("init tail error:", err)
		return
	}
	// tail file in tailPtr.lines
	for true {
		message, ok := <-tailPtr.Lines
		if !ok {
			fmt.Printf("tail file close reopen, filename:%s\n", path)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// fmt.Println("tail message is:", message.Text)
		tailJson := make(map[string]string)
		strBytes := []byte(message.Text)
		encodedStr := base64.StdEncoding.EncodeToString(strBytes)
		tailJson["message"] = encodedStr
		tailJson["path"] = path
		tailJson["host"] = HostIP
		tailJsonString, err := json.Marshal(tailJson)
		if err == nil {
			tailMessage <- tailJsonString
		} else {
			fmt.Println("tail json message error:", err)
		}
		if _,ok := tails[path]; !ok {
			fmt.Println("no client tail this file!")
			tailPtr.Cleanup()
			// break goroutine
			break
		}
	}
}

// Is file path is Exist
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// run system command
func RunCmdFunc(command string, args []string) (error, string, string, int) {
	cmd := execute.ExecTask{
		Command: command,
		Args:    args,
		Shell:   true,
	}
	res, err := cmd.Execute()

	return err, res.Stdout, res.Stderr, res.ExitCode
}

// the host IP you can config it in tail_agent.conf like HOST_IP : 192.168.1.1
const ConfigFile = "conf.yml"
var HostIP string
var TransitServer string

func main() {
	conf, err:= GetAgentConf(ConfigFile)
	if err == nil {
		HostIP = conf.HostIP
		TransitServer = conf.TransitServer
	} else {
		if len(os.Args) < 3 {
			log.Fatal("Please config conf.yaml or input host and transit server, e.g. tail_agent 192.168.2.1 192.168.2.19:8001")
		}
		HostIP = os.Args[1]
		TransitServer = os.Args[2]
	}

	closeWebsocket := false
	wsUrl := fmt.Sprintf("ws://%s/v1/tail/file/agent/ws", TransitServer)
	origin := fmt.Sprintf("http://%s", TransitServer)
	ws, err := websocket.Dial(wsUrl, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()
	go func() {
		if err != nil {
			log.Fatal(err)
		}
		var message = make([]byte, 50000)
		for {
			m, err := ws.Read(message)
			if err != nil {
				_ = ws.Close()
				log.Fatal(err)
			}
			str := string(message[:m])
			var reqInfo = make(map[string]interface{})
			_ = json.Unmarshal([]byte(str),&reqInfo)
			var tailPath string
			var command string
			reqType := reqInfo["type"].(string)
			tailNew := true
			if reqType == "open" {
				tailPath = reqInfo["path"].(string)
				exist,_  := PathExists(tailPath)
				if !exist {
					mes,_ := json.Marshal(map[string]interface{}{
						"message": fmt.Sprintf("you tailing file path [%s] is not exist on %s",tailPath,HostIP),
						"host": HostIP,
						"path": tailPath,
						"type":"error",
					})
					_,_ = ws.Write(mes)
				} else {
					for p, _ := range tails {
						if p == tailPath {
							tailNew = false
						}
					}
					if tailNew {
						fmt.Println("start new tail file:",tailPath)
						if err == nil {
							tails[tailPath] = true
							go TailFile(tailPath)
						} else {
							fmt.Println("tail file err:", err)
						}
					}
				}
			} else if reqType == "rrcreq" {
				id := reqInfo["id"].(string)
				command = strings.Split(reqInfo["cmd"].(string)," ")[0]
				args := strings.Split(reqInfo["cmd"].(string)," ")[1:]
				host := reqInfo["host"].(string)
				_,stdout,stderr,_ := RunCmdFunc(command, args)
				strBytes := []byte(stdout)
				encodedStr := base64.StdEncoding.EncodeToString(strBytes)
				res,_ := json.Marshal(map[string]interface{}{
					"timestamp":time.Now().Unix(),
					"stdout": encodedStr,
					"stderr":stderr,
					"host": host,
					"cmd": reqInfo["cmd"].(string),
					"id": id,
					"type":"rrcres",
				})
				ws.Write(res)
			} else {
				delete(tails, tailPath)
			}
			// fmt.Printf("Receive: %s\n", message[:m])
		}
	}()

	go func() {
		message := []byte{}
		for {
			if closeWebsocket {
				break
			}
			message = <- tailMessage
			tailJson := make(map[string]interface{})
			_ = json.Unmarshal(message,&tailJson)
			mes,_ := json.Marshal(map[string]interface{}{
				"timestamp":time.Now().Unix(),
				"message":tailJson["message"].(string),
				"host":tailJson["host"].(string),
				"path":tailJson["path"].(string),
				"type":"message",
			})
			_,_ = ws.Write(mes)
		}
	}()
	i := make(chan int)
	<-i
}
