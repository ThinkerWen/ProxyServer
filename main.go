package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	mu          sync.Mutex
	proxyList   = [50]string{}
	freeIndex   = make(chan int, 50)
	ProxyApiUrl = "https://api.xiaoxiangdaili.com/ip/get?appKey=956913709144756224&appSecret=eTDUzAY1&cnt=1&wt=json"
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func checkProxy(proxy string) (isValid bool, timeOut float64) {
	proxyParts := strings.Split(proxy, ":")

	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", proxyParts[0], proxyParts[1]), time.Second*1)

	if err != nil {
		return false, -1
	}
	defer func(conn net.Conn) { _ = conn.Close() }(conn)
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
		select {
		case index := <-freeIndex:
			log.Printf("add proxy %v\n", proxy)
			proxyList[index] = proxy
			continue
		case <-time.NewTimer(time.Microsecond * 500).C:
			log.Println("Proxy is full")
			return
		}
	}
}

func addSomeProxies() {
	if len(freeIndex) == 0 {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Printf("fetchUpstreamProxy %v\n", err)
		}
	}()
	response, err := http.Get(ProxyApiUrl)
	if err != nil {
		return
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	var mapResult map[string]interface{}
	err = json.Unmarshal(body, &mapResult)
	if err != nil {
		log.Printf("JsonToMapDemo err: %v\n", err)
		return
	}
	code := int(mapResult["code"].(float64))
	if code == 200 {
		result := mapResult["data"]
		if proxyList, ok := result.([]interface{}); ok {
			setGlobalProxyList(proxyList)
		} else {
			log.Printf("convert field %v\n", result)
		}
	}
}

func getRandomProxy() string {
	proxy := ""
	for i := 0; i < 50 && proxy == ""; i++ {
		proxy = proxyList[rand.Intn(50)]
	}
	return proxy
}

func forward(conn net.Conn, remoteAddr string) {
	client, err := net.DialTimeout("tcp", remoteAddr, time.Second*time.Duration(1))
	if err != nil {
		log.Printf("Dial failed: %v", err)
		for index := 0; index < 50; index++ {
			if proxyList[index] == remoteAddr {
				log.Printf("remove inactive proxy %s\n", proxyList[index])
				mu.Lock()
				proxyList[index] = ""
				freeIndex <- index
				mu.Unlock()
			}
		}
		forward(conn, getRandomProxy())
		return
	}
	log.Printf("Forwarding from %v to %v\n", conn.LocalAddr(), client.RemoteAddr())
	go func() {
		defer func(client net.Conn) { _ = client.Close() }(client)
		defer func(conn net.Conn) { _ = conn.Close() }(conn)
		_, _ = io.Copy(client, conn)
	}()
	go func() {
		defer func(client net.Conn) { _ = client.Close() }(client)
		defer func(conn net.Conn) { _ = conn.Close() }(conn)
		_, _ = io.Copy(conn, client)
	}()
}

func checkProxyList() {
	log.Println("启动代理检测协程")
	for {
		log.Printf("代理检测中，代理池容量: %d\n", len(proxyList)-len(freeIndex))
		for i := 0; i < len(proxyList) && proxyList[i] != ""; i++ {
			go func(index int) {
				valid, _ := checkProxy(proxyList[index])
				if !valid {
					log.Printf("remove inactive proxy %s", proxyList[index])
					mu.Lock()
					defer mu.Unlock()
					proxyList[index] = ""
					freeIndex <- index
				}
			}(i)
		}
		time.Sleep(time.Second * 7)
	}
}

func main() {
	for i := 0; i < 50; i++ {
		freeIndex <- i
	}

	go checkProxyList()
	go func() {
		for {
			addSomeProxies()
			time.Sleep(10 * time.Second)
		}
	}()

	rand.Seed(time.Now().UnixNano())
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
		px := getRandomProxy()
		if px == "" {
			px = "127.0.0.1:60001"
		}
		go forward(conn, px)
	}
}
