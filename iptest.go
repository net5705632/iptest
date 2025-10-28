import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	@@ -19,37 +17,30 @@ import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	requestURL  = "speed.cloudflare.com/cdn-cgi/trace" // 请求trace URL
	timeout     = 1 * time.Second                      // 超时时间
	maxDuration = 2 * time.Second                      // 最大持续时间
)

var (
	File         = flag.String("file", "ip.txt", "IP地址文件名称,格式为 ip port ,就是IP和端口之间用空格隔开")       // IP地址文件名称
	outFile      = flag.String("outfile", "ip.csv", "输出文件名称")                                  // 输出文件名称
	maxThreads   = flag.Int("max", 100, "并发请求最大协程数")                                           // 最大协程数
	speedTest    = flag.Int("speedtest", 5, "下载测速协程数量,设为0禁用测速")                                // 下载测速协程数量
	speedTestURL = flag.String("url", "speed.cloudflare.com/__down?bytes=500000000", "测速文件地址") // 测速文件地址
	enableTLS    = flag.Bool("tls", true, "是否启用TLS")                                           // TLS是否启用
	delay = flag.Int("delay", 0, "延迟阈值(ms)，默认为0禁用延迟过滤")                           // 默认0，禁用过滤
)

type result struct {
	ip          string        // IP地址
	port        int           // 端口
	dataCenter  string        // 数据中心
	locCode    string        // 源IP位置
	region      string        // 地区
	city        string        // 城市
	region_zh      string        // 地区
	country        string        // 国家
	city_zh      string        // 城市
	emoji      string        // 国旗
	latency     string        // 延迟
	tcpDuration time.Duration // TCP请求延迟
}
	@@ -66,10 +57,6 @@ type location struct {
	Cca2   string  `json:"cca2"`
	Region string  `json:"region"`
	City   string  `json:"city"`
	Region_zh string  `json:"region_zh"`
	Country   string  `json:"country"`
	City_zh string  `json:"city_zh"`
	Emoji   string  `json:"emoji"`
}

// 尝试提升文件描述符的上限
	@@ -86,26 +73,33 @@ func increaseMaxOpenFiles() {

func main() {
	flag.Parse()
	var validCount int32 // 有效IP计数器

	startTime := time.Now()
	osType := runtime.GOOS
	if osType == "linux" {
		increaseMaxOpenFiles()
	}

	var locations []location
	if _, err := os.Stat("locations.json"); os.IsNotExist(err) {
		fmt.Println("本地 locations.json 不存在\n正在从 https://locations-adw.pages.dev/ 下载 locations.json")
		resp, err := http.Get("https://locations-adw.pages.dev/")
		if err != nil {
			fmt.Printf("无法从URL中获取JSON: %v\n", err)
			return
		}

		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("无法读取响应体: %v\n", err)
			return
	@@ -121,7 +115,12 @@ func main() {
			fmt.Printf("无法创建文件: %v\n", err)
			return
		}
		defer file.Close()

		_, err = file.Write(body)
		if err != nil {
	@@ -135,9 +134,14 @@ func main() {
			fmt.Printf("无法打开文件: %v\n", err)
			return
		}
		defer file.Close()

		body, err := ioutil.ReadAll(file)
		if err != nil {
			fmt.Printf("无法读取文件: %v\n", err)
			return
	@@ -208,13 +212,14 @@ func main() {
			if err != nil {
				return
			}
			defer conn.Close()

			tcpDuration := time.Since(start)
			if *delay > 0 && tcpDuration.Milliseconds() > int64(*delay) {
			    return // 超过延迟阈值直接返回（仅在delay>0时生效）
			}

			start = time.Now()

			client := http.Client{
	@@ -232,7 +237,7 @@ func main() {
			} else {
				protocol = "http://"
			}
			requestURL := protocol + requestURL

			req, _ := http.NewRequest("GET", requestURL, nil)

	@@ -249,48 +254,27 @@ func main() {
				return
			}

			defer resp.Body.Close()
			buf := &bytes.Buffer{}
			// 创建一个读取操作的超时
			timeout := time.After(maxDuration)
			// 使用一个 goroutine 来读取响应体
			done := make(chan bool)
			errChan := make(chan error)
			go func() {
				_, err := io.Copy(buf, resp.Body)
				done <- true
				errChan <- err
				if err != nil {
					return
				}
			}()
			// 等待读取操作完成或者超时
			select {
			case <-done:
				// 读取操作完成
			case <-timeout:
				// 读取操作超时
				return
			}

			body := buf
			err = <-errChan
			if err != nil {
				return
			}
			if strings.Contains(body.String(), "uag=Mozilla/5.0") {
				if matches := regexp.MustCompile(`colo=([A-Z]+)[\s\S]*?loc=([A-Z]+)`).FindStringSubmatch(body.String()); len(matches) > 2 {
					dataCenter := matches[1]
					locCode := matches[2]
					loc, ok := locationMap[dataCenter]
					// 记录通过延迟检查的有效IP
					atomic.AddInt32(&validCount, 1)
					if ok {
						fmt.Printf("发现有效IP %s 端口 %d 位置信息 %s 延迟 %d 毫秒\n", ipAddr, port, loc.City_zh, tcpDuration.Milliseconds())
						resultChan <- result{ipAddr, port, dataCenter, locCode, loc.Region, loc.City, loc.Region_zh, loc.Country, loc.City_zh, loc.Emoji, fmt.Sprintf("%d ms", tcpDuration.Milliseconds()), tcpDuration}
					} else {
						fmt.Printf("发现有效IP %s 端口 %d 位置信息未知 延迟 %d 毫秒\n", ipAddr, port, tcpDuration.Milliseconds())
						resultChan <- result{ipAddr, port, dataCenter, locCode, "", "", "", "", "", "", fmt.Sprintf("%d ms", tcpDuration.Milliseconds()), tcpDuration}
					}
				}
			}
	@@ -301,14 +285,11 @@ func main() {
	close(resultChan)

	if len(resultChan) == 0 {
		// 清除输出内容
		fmt.Print("\033[2J")
		fmt.Println("没有发现有效的IP")
		return
	}
	var results []speedtestresult
	if *speedTest > 0 {
	    fmt.Printf("找到符合条件的ip 共%d个\n", atomic.LoadInt32(&validCount))
		fmt.Printf("开始测速\n")
		var wg2 sync.WaitGroup
		wg2.Add(*speedTest)
	@@ -358,26 +339,34 @@ func main() {
		fmt.Printf("无法创建文件: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if *speedTest > 0 {
		writer.Write([]string{"IP地址", "端口", "TLS", "数据中心", "源IP位置", "地区", "城市", "地区(中文)", "国家", "城市(中文)", "国旗", "网络延迟", "下载速度"})
	} else {
		writer.Write([]string{"IP地址", "端口", "TLS", "数据中心", "源IP位置", "地区", "城市", "地区(中文)", "国家", "城市(中文)", "国旗", "网络延迟"})
	}
	for _, res := range results {
		if *speedTest > 0 {
			writer.Write([]string{res.result.ip, strconv.Itoa(res.result.port), strconv.FormatBool(*enableTLS), res.result.dataCenter, res.result.locCode, res.result.region, res.result.city, res.result.region_zh, res.result.country, res.result.city_zh, res.result.emoji, res.result.latency, fmt.Sprintf("%.0f kB/s", res.downloadSpeed)})
		} else {
			writer.Write([]string{res.result.ip, strconv.Itoa(res.result.port), strconv.FormatBool(*enableTLS), res.result.dataCenter, res.result.locCode, res.result.region, res.result.city, res.result.region_zh, res.result.country, res.result.city_zh, res.result.emoji, res.result.latency})
		}
	}

	writer.Flush()
	// 清除输出内容
	fmt.Print("\033[2J")
	fmt.Printf("有效IP数量: %d | 成功将结果写入文件 %s，耗时 %d秒\n", atomic.LoadInt32(&validCount), *outFile, time.Since(startTime)/time.Second)
}

// 从文件中读取IP地址和端口
	@@ -386,7 +375,12 @@ func readIPs(File string) ([]string, error) {
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
	@@ -411,16 +405,6 @@ func readIPs(File string) ([]string, error) {
	return ips, scanner.Err()
}

// inc函数实现ip地址自增
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// 测速函数
func getDownloadSpeed(ip string, port int) float64 {
	var protocol string
	@@ -443,7 +427,12 @@ func getDownloadSpeed(ip string, port int) float64 {
	if err != nil {
		return 0
	}
	defer conn.Close()

	fmt.Printf("正在测试IP %s 端口 %d\n", ip, port)
	startTime := time.Now()
	@@ -464,7 +453,12 @@ func getDownloadSpeed(ip string, port int) float64 {
		fmt.Printf("IP %s 端口 %d 测速无效\n", ip, port)
		return 0
	}
	defer resp.Body.Close()

	// 复制响应体到/dev/null，并计算下载速度
	written, _ := io.Copy(io.Discard, resp.Body)
