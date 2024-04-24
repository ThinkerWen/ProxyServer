# ProxyServer

将API代理构建成隧道代理池

<br>

## 介绍
此项目用于将API代理构建成隧道代理池，即通过一个IP和端口来自动随机取出代理池中的代理。

API代理即：
* 通过API来获取代理，返回值是代理列表包含1~n个代理
* 例如：`[{"ip":"xxx.xxx.xxx.xxx","port":xx},{"ip":"xxx.xxx.xxx.xxx","port":xx}]`

这样的代理不方便利用，在代码中通常需要二次操作，这个项目便能完美解决这个问题。

<br>

## 安装
1.克隆本项目
```bash
git clone https://github.com/ThinkerWen/ProxyServer.git
```
2.修改`main.go`文件中的`getProxies()`函数，将它改为您的API代理的获取函数即可。（项目中的示例使用的是[小象代理](https://www.xiaoxiangdaili.com/)

```bash
go get ProxyServer    # 下载依赖项
go build ProxyServer  # 编译可执行程序
./ProxyServer         # 运行
```

<br>

## 使用
在后台启动本项目后，只需配置代理地址为`127.0.0.1:12315`即可（在[配置文件](#配置文件)中可修改），以下用Python做示例：
```python
import requests

proxies = {
    "http": "http://127.0.0.1:12315",
    "https": "http://127.0.0.1:12315"
}
requests.get("http://example.com", proxies=proxies)
```

<br>

## 配置文件
配置文件一般默认即可
```json
{
  "bind_ip": "",                  # 代理服务绑定IP
  "bind_port": 12315,             # 代理服务绑定端口
  "proxy_max_retry": 10,          # 代理IP重试次数（超过后此次请求废弃）
  "proxy_expire_time": 50,        # 代理IP过期时间（超过后此IP移除代理池）
  "proxy_pool_length": 50,        # 代理IP池大小
  "proxy_connect_time_out": 1,    # 代理IP连接超时时间（超过后此IP移除代理池）
  "check_proxy_time_period": 10,  # 代理IP有效性检测间隔
  "refresh_proxy_time_period": 10 # API代理的获取间隔
}
```