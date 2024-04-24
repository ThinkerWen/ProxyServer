package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	config      Config
	mu          sync.Mutex
	proxyList   []string
	freeIndex   chan int
	ProxyApiUrl = "https://api.xiaoxiangdaili.com/ip/get?appKey=956913709144756224&appSecret=eTDUzAY1&cnt=1&wt=json"
)

type Config struct {
	BindIP                 string `json:"bind_ip"`
	BindPort               int    `json:"bind_port"`
	ProxyMaxRetry          int    `json:"proxy_max_retry"`
	ProxyExpireTime        int64  `json:"proxy_expire_time"`
	ProxyPoolLength        int    `json:"proxy_pool_length"`
	ProxyConnectTimeOut    int    `json:"proxy_connect_time_out"`
	CheckProxyTimePeriod   int    `json:"check_proxy_time_period"`
	RefreshProxyTimePeriod int    `json:"refresh_proxy_time_period"`
}

func init() {
	jsonBytes, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatal("Error reading JSON file:", err)
	}
	if err = json.Unmarshal(jsonBytes, &config); err != nil {
		log.Fatal("Error decoding JSON:", err)
	}
	proxyList = make([]string, config.ProxyPoolLength)
	freeIndex = make(chan int, config.ProxyPoolLength)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func checkProxy(proxy string) bool {
	proxyParts := strings.Split(proxy, ":")

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", proxyParts[0], proxyParts[1]), time.Second*time.Duration(config.ProxyConnectTimeOut))
	if err != nil {
		return false
	}
	defer func(conn net.Conn) { _ = conn.Close() }(conn)
	return true
}

func setProxyList(pl []string) {
	if len(pl) == 0 {
		return
	}
	for _, proxy := range pl {
		select {
		case index := <-freeIndex:
			log.Printf("add proxy %v\n", proxy)
			proxyList[index] = fmt.Sprintf("%s#%d", proxy, time.Now().Unix()+config.ProxyExpireTime)
		case <-time.NewTimer(time.Microsecond * 500).C:
			log.Println("Proxy is full")
			return
		}
	}
}

func getProxies() {
	if len(freeIndex) == 0 {
		return
	}

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
	if err = json.Unmarshal(body, &mapResult); err != nil {
		log.Printf("JsonToMapDemo err: %v\n", err)
		return
	}
	if int(mapResult["code"].(float64)) == 200 {
		result := mapResult["data"]
		proxies := make([]string, 0)
		for _, proxyDict := range result.([]interface{}) {
			proxy := proxyDict.(map[string]interface{})
			proxies = append(proxies, fmt.Sprintf("%s:%d", proxy["ip"].(string), int(proxy["port"].(float64))))
		}
		setProxyList(proxies)
	}
}

func getRandomProxy() string {
	for i := 0; i < 10; i++ {
		if proxy := proxyList[rand.Intn(config.ProxyPoolLength)]; proxy != "" {
			return strings.Split(proxy, "#")[0]
		}
	}
	for _, proxy := range proxyList {
		if proxy != "" {
			return strings.Split(proxy, "#")[0]
		}
	}
	return ""
}

func forward(conn net.Conn, remote string, retry int) {
	client, err := net.DialTimeout("tcp", remote, time.Second*time.Duration(config.ProxyConnectTimeOut))
	if err != nil {
		retry--
		log.Printf("Dial failed: %v", err)
		if retry > 0 {
			forward(conn, getRandomProxy(), retry)
		}
		return
	}
	log.Printf("Forwarding from %v to %v\n", conn.LocalAddr(), client.RemoteAddr())

	ioCopy := func(src net.Conn, dst net.Conn) {
		_, _ = io.Copy(src, dst)
		_ = src.Close()
	}
	go ioCopy(conn, client)
	go ioCopy(client, conn)
}

func checkProxyList() {
	log.Printf("代理检测中，代理池容量: %d\n", len(proxyList)-len(freeIndex))
	for i := 0; i < len(proxyList) && proxyList[i] != ""; i++ {
		go func(index int) {
			proxy := strings.Split(proxyList[index], "#")[0]
			period := strings.Split(proxyList[index], "#")[1]
			if period >= strconv.FormatInt(time.Now().Unix(), 10) || checkProxy(proxy) {
				mu.Lock()
				defer mu.Unlock()
				log.Printf("remove inactive proxy %s", proxy)
				proxyList[index] = ""
				freeIndex <- index
			}
		}(i)
	}
	time.Sleep(time.Second * time.Duration(config.CheckProxyTimePeriod))
	checkProxyList()
}

func main() {
	for i := 0; i < config.ProxyPoolLength; i++ {
		freeIndex <- i
	}

	go checkProxyList()
	go func() {
		for {
			getProxies()
			time.Sleep(time.Duration(config.RefreshProxyTimePeriod) * time.Second)
		}
	}()

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", config.BindIP, config.BindPort))
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
		go forward(conn, px, config.ProxyMaxRetry)
	}
}
