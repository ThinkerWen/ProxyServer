package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/CreditTone/aslist"
	colorfulog "github.com/CreditTone/colorfulog"
	"github.com/lxzan/hasaki"
)

var (
	ProxyApiUrl = "https://api.xiaoxiangdaili.com/ip/get?appKey=956913709144756224&appSecret=eTDUzAY1&cnt=1&wt=json"
	proxyList   = aslist.NewAsList()
	invalid     = make(chan string)
)

func checkProxy(proxy string) (isValid bool, timeOut float64) {
	proxyParts := strings.Split(proxy, ":")

	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", proxyParts[0], proxyParts[1]), time.Second*5)

	if err != nil {
		return false, -1
	}
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			colorfulog.Info(err)
		}
	}(conn)

	_, err = conn.Write([]byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", "8.8.8.8")))
	if err != nil {
		return false, -1
	}

	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return false, -1
	}

	elapsedTime := time.Since(startTime)
	return true, elapsedTime.Seconds()
}

func setGlobalProxyList(pl []interface{}) {
	if len(pl) == 0 {
		return
	}
	for _, p := range pl {
		m := p.(map[string]interface{})
		proxy := fmt.Sprintf("%s:%v", m["ip"], m["port"])
		colorfulog.Infof("add proxy %v", proxy)
		proxyList.LeftPush(proxy)
	}
	for proxyList.Length() > 50 {
		colorfulog.Infof("remove proxy %v", proxyList.RightPop())
	}
}

func updateUpstreamProxy() {
	defer func() {
		if err := recover(); err != nil {
			colorfulog.Warnf("fetchUpstreamProxy %v", err)
		}
	}()
	body, err := hasaki.Get(ProxyApiUrl).Send(nil).GetBody()
	if err != nil {
		return
	}
	var mapResult map[string]interface{}
	err = json.Unmarshal(body, &mapResult)
	if err != nil {
		colorfulog.Warnf("JsonToMapDemo err: %v ", err)
		return
	}
	code := int(mapResult["code"].(float64))
	if code == 200 {
		result := mapResult["data"]
		if proxyList, ok := result.([]interface{}); ok {
			setGlobalProxyList(proxyList)
		} else {
			colorfulog.Warnf("convert field %v", result)
		}
	}
}

func forward(conn net.Conn, remoteAddr string) {
	client, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("Dial failed: %v", err)
		defer func(conn net.Conn) {
			_ = conn.Close()
		}(conn)
		return
	}
	log.Printf("Forwarding from %v to %v\n", conn.LocalAddr(), client.RemoteAddr())
	go func() {
		defer func(client net.Conn) {
			_ = client.Close()
		}(client)
		defer func(conn net.Conn) {
			_ = conn.Close()
		}(conn)
		_, _ = io.Copy(client, conn)
	}()
	go func() {
		defer func(client net.Conn) {
			_ = client.Close()
		}(client)
		defer func(conn net.Conn) {
			_ = conn.Close()
		}(conn)
		_, _ = io.Copy(conn, client)
	}()
}

func updateProxyList() {
	colorfulog.Info("启动代理更新协程")
	for {
		updateUpstreamProxy()
		time.Sleep(time.Second * 10)
	}
}

func checkProxyList() {
	colorfulog.Info("启动代理检测协程")
	for {
		colorfulog.Info("代理检测中，代理池容量:", proxyList.Length())
		for i := 0; i < proxyList.Length(); i++ {
			index := i
			go func() {
				valid, timeOut := checkProxy(proxyList.Get(index).(string))
				if !valid || timeOut > 1 && proxyList.Get(index) != nil {
					invalid <- proxyList.Get(index).(string)
				}
			}()
		}
		time.Sleep(time.Second * 10)
	}
}

func removeProxy() {
	for proxy := range invalid {
		clearFunc := func(index int, item interface{}) bool {
			return item != nil && index == index && item.(string) == proxy
		}
		proxyList.ClearTargets(clearFunc)
		colorfulog.Infof("remove proxy %s", proxy)
	}
}

func main() {

	//启动代理更新线程
	go updateProxyList()
	go checkProxyList()
	go removeProxy()

	listener, err := net.Listen("tcp", ":12315")
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("ERROR: failed to accept listener: %v", err)
		}
		log.Printf("Accepted connection from %v\n", conn.RemoteAddr().String())
		var px interface{}
		px = proxyList.RandomGet()
		if px == nil {
			//随便设置个无效的代理
			px = "127.0.0.1:60001"
		}
		go forward(conn, px.(string))
	}
}
