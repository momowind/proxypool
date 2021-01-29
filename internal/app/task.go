package app

import (
	"github.com/momowind/proxypool/config"
	"github.com/momowind/proxypool/log"
	"github.com/momowind/proxypool/pkg/healthcheck"
	"sync"
	"time"

	"github.com/momowind/proxypool/internal/cache"
	"github.com/momowind/proxypool/internal/database"
	"github.com/momowind/proxypool/pkg/provider"
	"github.com/momowind/proxypool/pkg/proxy"
)

var location, _ = time.LoadLocation("PRC")

func CrawlGo() {
	wg := &sync.WaitGroup{}
	var pc = make(chan proxy.Proxy)
	for _, g := range Getters {
		wg.Add(1)
		go g.Get2ChanWG(pc, wg)
	}
	proxies := cache.GetProxies("allproxies")
	dbProxies := database.GetAllProxies()
	// Show last time result when launch
	if proxies == nil && dbProxies != nil {
		cache.SetProxies("proxies", dbProxies)
		cache.LastCrawlTime = "抓取中，已载入上次数据库数据"
		log.Infoln("Database: loaded")
	}
	if dbProxies != nil {
		proxies = dbProxies.UniqAppendProxyList(proxies)
	}
	if proxies == nil {
		proxies = make(proxy.ProxyList, 0)
	}

	go func() {
		wg.Wait()
		close(pc)
	}() // Note: 为何并发？可以一边抓取一边读取而非抓完再读
	// for 用于阻塞goroutine
	for p := range pc { // Note: pc关闭后不能发送数据可以读取剩余数�?
		if p != nil {
			proxies = proxies.UniqAppendProxy(p)
		}
	}

	proxies = proxies.Derive()
	log.Infoln("CrawlGo unique proxy count: %d", len(proxies))

	// Clean Clash unsupported proxy because health check depends on clash
	proxies = provider.Clash{
		provider.Base{
			Proxies: &proxies,
		},
	}.CleanProxies()
	log.Infoln("CrawlGo clash supported proxy count: %d", len(proxies))

	cache.SetProxies("allproxies", proxies)
	cache.AllProxiesCount = proxies.Len()
	log.Infoln("AllProxiesCount: %d", cache.AllProxiesCount)
	cache.SSProxiesCount = proxies.TypeLen("ss")
	log.Infoln("SSProxiesCount: %d", cache.SSProxiesCount)
	cache.SSRProxiesCount = proxies.TypeLen("ssr")
	log.Infoln("SSRProxiesCount: %d", cache.SSRProxiesCount)
	cache.VmessProxiesCount = proxies.TypeLen("vmess")
	log.Infoln("VmessProxiesCount: %d", cache.VmessProxiesCount)
	cache.TrojanProxiesCount = proxies.TypeLen("trojan")
	log.Infoln("TrojanProxiesCount: %d", cache.TrojanProxiesCount)
	cache.LastCrawlTime = time.Now().In(location).Format("2006-01-02 15:04:05")

	// 节点可用性检测，使用batchsize不能降低内存占用，只是为了看性能
	log.Infoln("Now proceed proxy health check...")
	b := 1000
	round := len(proxies) / b
	okproxies := make(proxy.ProxyList, 0)
	for i := 0; i < round; i++ {
		okproxies = append(okproxies, healthcheck.CleanBadProxiesWithGrpool(proxies[i*b:(i+1)*b])...)
		log.Infoln("\tChecking round: %d", i)
	}
	okproxies = append(okproxies, healthcheck.CleanBadProxiesWithGrpool(proxies[round*b:])...)
	proxies = okproxies

	log.Infoln("CrawlGo clash usable proxy count: %d", len(proxies))

	// 重命名节点名称为类似US_01的格式，并按国家排序
	proxies.NameSetCounrty().Sort().NameAddIndex()
	log.Infoln("Proxy rename DONE!")

	// 可用节点存储
	cache.SetProxies("proxies", proxies)
	cache.UsefullProxiesCount = proxies.Len()
	database.SaveProxyList(proxies)
	database.ClearOldItems()

	log.Infoln("Usablility checking done. Open %s to check", config.Config.Domain+":"+config.Config.Port)

	// 测�?
	speedTestNew(proxies)
	cache.SetString("clashproxies", provider.Clash{
		provider.Base{
			Proxies: &proxies,
		},
	}.Provide()) // update static string provider
	cache.SetString("surgeproxies", provider.Surge{
		provider.Base{
			Proxies: &proxies,
		},
	}.Provide())
}

// Speed test for new proxies
func speedTestNew(proxies proxy.ProxyList) {
	if config.Config.SpeedTest {
		cache.IsSpeedTest = "已开�?"
		if config.Config.Timeout > 0 {
			healthcheck.SpeedTimeout = time.Second * time.Duration(config.Config.Timeout)
		}
		healthcheck.SpeedTestNew(proxies, config.Config.Connection)
	} else {
		cache.IsSpeedTest = "未开�?"
	}
}

// Speed test for all proxies in proxy.ProxyList
func SpeedTest(proxies proxy.ProxyList) {
	if config.Config.SpeedTest {
		cache.IsSpeedTest = "已开�?"
		if config.Config.Timeout > 0 {
			healthcheck.SpeedTimeout = time.Second * time.Duration(config.Config.Timeout)
		}
		healthcheck.SpeedTestAll(proxies, config.Config.Connection)
	} else {
		cache.IsSpeedTest = "未开�?"
	}
}
