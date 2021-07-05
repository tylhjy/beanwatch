# beanwatch
Beanwatch is a small tool that can monitor log files on remote hosts in real time and execute remote host commands. The network transmission protocol is websocket

### Architecture
    [agent 192.168.1.2] [agent 192.168.1.3] [agent 192.168.1.4]
        |                       |                   |
                  |             |          |
                          |     |      |
                                |
                     (server 192.168.1.10:8001)
                                |
                                |
                       {client:10.110.10.23}
       
### agent
agent.go is an agent running on a remote host. Once this agent runs, it will automatically register with the server, and then monitor the client's requests forwarded by the    server in real time. Currently, two types of requests are received, one is to monitor log files like the tail command, and the other is to execute system commands. Note: The agent is best to run in the background, such as using nohup to run.The configuration file is: conf.yml. It contains the IP address of the host where the agent is located and the host:port of the server

### server
server.go is a secondary school server, used to establish a websocket connection between the remote host agent and client, and forward the message correctly.Note: The server runs on port 8001 by default, of course, you can also specify the port like this: go run server.go 8002

### rrc client
rrc.go is a client that executes system commands on a remote host. The calling method is as follows:go run rrc.go 192.168.1.123 ls /root. The configuration file is client_conf.yml. It contains the host:port of the server
### wrf client
wrf.go wrf.go is a command to monitor log files on remote hosts in real time. The execution command is as follows: go run wrf.go 192.168.1.123 /var/log/messages. The configuration file is client_conf.yml. It contains the host:port of the server
