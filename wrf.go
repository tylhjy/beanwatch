/**
 * @Author Tian Yang
 * @license: (C) Copyright 2013-2021, Tian Yang.AllRightsReserved.
 * @Description: a kind of beanwatch client for watch log file on remote host in real time
 * @Date: 16:08 2021/6/30
 * @Software: BeanWatch
 * @Version: 1.0
 **/
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ClientConf struct {
	TransitHost string `yaml:"transit_host"`
}

const ClientConfigFile = "client_conf.yml"
var TransitHost string

// Get Config
func GetClientConf(configFile string) (*ClientConf,error) {
	var c ClientConf
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

func GetExcPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))
	ret := path[:index]
	return ret
}

func main() {
	confDir := GetExcPath()
	conf, err:= GetClientConf(fmt.Sprintf("%s%s%s",confDir, string(os.PathSeparator),ClientConfigFile))
	if err == nil {
		TransitHost = conf.TransitHost
		if len(os.Args) < 3 {
			log.Fatal("Please input host and path that you want to tail, e.g. wrf 192.168.1.3 D://test.txt")
		}
	} else {
		fmt.Println("Please config conf.yaml!")
		log.Fatal(err)
	}
	hostIP := os.Args[1]
	filePath := os.Args[2]
	wsUrl := fmt.Sprintf("ws://%s/v1/tail/file/client/%s/ws?path=%s",TransitHost,hostIP,filePath)
	origin := fmt.Sprintf("http://%s",TransitHost)
	ws, err := websocket.Dial(wsUrl, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()
	var i = make(chan int)
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
				i <- 1
				break
			} else {
				timestamp := int64(mesJson["timestamp"].(float64))
				tm := time.Unix(timestamp, 0)
				decoded, _ := base64.StdEncoding.DecodeString(mesJson["message"].(string))
				tailMessage := string(decoded)
				//fmt.Println(tailMessage)
				fmt.Println(fmt.Sprintf("%s: %s",tm.Format("2006-01-02 03:04:05 PM"),tailMessage))
			}
		}
	}()
	<-i
}
