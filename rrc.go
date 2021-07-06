/**
 * @Author Tian Yang
 * @license: (C) Copyright 2013-2021, Tian Yang.AllRightsReserved.
 * @Description: a kind of beanwatch client for run system command on remote host
 * @Date: 11:12 2021/7/3
 * @Software: BeanWatch
 * @Version: 1.0
 **/
package main

import (
	"encoding/base64"
	"encoding/json"
	"golang.org/x/net/websocket"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"fmt"
)

type RrcConf struct {
	TransitHost string `yaml:"transit_host"`
}

type RrcResponse struct {
	Success bool   `json:"success"`
	StdOut  string `json:"stdout"`
	StdErr  string `json:"stderr"`
	Error   string `json:"error"`
}

const RrcConfigFile = "client_conf.yml"
var TransitNode string

// Get Config
func GetRrcConf(configFile string) (*RrcConf,error) {
	var c RrcConf
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

// Get rrc command path
func GetRrcPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))
	ret := path[:index]
	return ret
}

func main() {
	confDir := GetRrcPath()
	conf, err := GetRrcConf(fmt.Sprintf("%s%s%s",confDir, string(os.PathSeparator),RrcConfigFile))
	if err == nil {
		TransitNode = conf.TransitHost
		if len(os.Args) < 3 {
			log.Fatal("Please input host and path that you want to tail, e.g. rail 192.168.2.1 D://456.txt")
		}
	} else {
		fmt.Println("Please config conf.yaml!")
		log.Fatal(err)
	}
	hostIP := os.Args[1]
	cmd := strings.Join(os.Args[2:]," ")
	strBytes := []byte(cmd)
	encodedCmd := base64.StdEncoding.EncodeToString(strBytes)
	wsUrl := fmt.Sprintf("ws://%s/v1/run/cmd/client/%s/ws?cmd=%s",TransitNode,hostIP,encodedCmd)
	origin := fmt.Sprintf("http://%s",TransitNode)
	ws, err := websocket.Dial(wsUrl, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()
	// send run command message
	_ ,err = ws.Write([]byte(cmd))
	if err != nil {
		log.Fatal("send cmd to remote failed!")
	}
	var rrcRes = make(chan RrcResponse)
	go func() {
		if err != nil {
			log.Fatal(err)
		}
		var message = make([]byte, 4096)
		var messageTotal []byte
		for {
			m, err := ws.Read(message)
			if err != nil {
				_ = ws.Close()
				log.Fatal(err)
			}
			// fmt.Printf("Receive: %s\n", message[:m])
			mesJson := make(map[string]interface{})
			if strings.Index(string(message[:m]),"{") >= 0 && strings.Index(string(message[:m]),"}") >= 0 {
				_ = json.Unmarshal(message[:m],&mesJson)
			} else if strings.Index(string(message[:m]),"{") >= 0 && strings.Index(string(message[:m]),"}") < 0 {
				messageTotal = []byte{}
				messageTotal = append(messageTotal,message[:m]...)
				continue
			} else if strings.Index(string(message[:m]),"{") < 0 && strings.Index(string(message[:m]),"}") < 0 {
				messageTotal = append(messageTotal,message[:m]...)
				continue
			} else {
				messageTotal = append(messageTotal,message[:m]...)
				_ = json.Unmarshal(messageTotal,&mesJson)
			}
			if mesJson["type"].(string) == "error" {
				fmt.Println(mesJson["message"].(string))
				_ = ws.Close()
				rrcRes <- RrcResponse{
					Success: false,
					StdOut:  "",
					StdErr:  "",
					Error: mesJson["message"].(string),
				}
			} else {
				timestamp := int64(mesJson["timestamp"].(float64))
				tm := time.Unix(timestamp, 0)
				decoded, _ := base64.StdEncoding.DecodeString(mesJson["stdout"].(string))
				stdOut := string(decoded)
				fmt.Println(tm)
				rrcRes <- RrcResponse{
					Success: true,
					StdOut:  stdOut,
					StdErr:  mesJson["stderr"].(string),
					Error: "",
				}
			}
			break
		}
	}()
	// wait for remote response
	response :=<- rrcRes
	if response.Success {
		if response.StdErr == "" {
			fmt.Println(response.StdOut)
		} else {
			fmt.Println(response.StdErr)
		}
	} else {
		fmt.Println("!server error!:",response.Error)
	}
}
