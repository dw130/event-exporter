package main


import (
	"github.com/go-redis/redis"
	"fmt"
	"flag"
	"github.com/golang/glog"
	"os"
	"time"
	//"strings"
	//"encoding/json"
	"sync"
	"regexp"
	"github.com/influxdata/influxdb/client/v2"
)

var (
	redisFlag     = flag.String("redis", "", "redis")
	influxdbFlag  = flag.String("influxdb", "", "String")
	interval      = flag.Int("flush interval",40,"")
	env           = flag.String("env","poc","")
)

var (
	allSG = []string{}
	mutex   sync.Mutex
)

var (
	ALLAPP = "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:applications:members"
	SG = "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:clusters:members"
	INSTANCE = "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:instances:members"
	PODSTATUS = "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:instances:attributes:kubernetes:instances:%v:status"
	POCPRO = "kubernetes:instances:poc-qcloud-sh*"
	REG = regexp.MustCompile("\"podStatus\":[^\"]*\"([^\"]+)")
)

func FetchD(client *redis.Client,inf  influxdb.Client) {
	mutex.Lock()
	defer mutex.Unlock()

	var cursor uint64
	var count int

	prefix := "kubernetes:instances:prod-qcloud-sh*"
	evv := *env
	if evv == "poc" {
		prefix = POCPRO
	}

	loop:
	for {
	    var keys []string
	    var err error
	    keys, cursor, err = client.SScan(INSTANCE, cursor, prefix, 500).Result()
	    allSG = append(allSG,keys...)
	    if err != nil {
	        glog.Infof("redis scan err:%v",err)
	        break loop
	    }
	    count += len(keys)
	    if cursor <= 0 {
	        break loop
	    }
	}
	glog.Infof("fetch total sg number:%v",count)
	fetchdetail(client,inf)
}

func fetchAll(client *redis.Client) {
	mutex.Lock()
	defer mutex.Unlock()
}

func fetchdetail(client *redis.Client, inf  influxdb.Client) {

	allMetric := map[string] map[string] [2]int{} 

	for k,_ := range allSG {

		all := strings.Split(allSG[k],":")
		var ver string
		var sg  string

		if len(all) != 0 {
			meta := all[ len(all) - 1 ]
			podList := strings.Split(meta,"-")
			if strings.Contains(allSG[k], "-v") == true {
				//canarydemo-canary-v019-9dl9d
				ll := len(podList)
				//podId := podList[  ll - 1 ]
				ver   = podList[  ll - 2 ]
				sg    = strings.Join( podList[ 0: ll - 2 ]  )
			} else {
				ver = "default"
				sg  =  strings.Join( podList[ 0: ll - 1 ]  )
			}

			_,ok := allMetric[sg]
			if ok == false {
				allMetric[sg] = map[string] [2]string{}
			}
			_,ok = allMetric[sg][ver]
			if ok == false {
				allMetric[sg][ver] = [2]string{0,0}
			}
			
		} else {
			continue
		}

		pod := fmt.Sprintf(PODSTATUS, allSG[k][21:])
		val, err := client.Get(pod).Result()
		if err != nil {
			glog.Infof("fetch pod status fail:%v",err)
			continue
		}


		ret := REG.FindAllStringSubmatch(val,1)
		if len(ret) {
			pods := ret[0][1]
			if pods == "RUNNING" {
				allMetric[sg][ver][0]  = allMetric[sg][ver][0] + 1
			} else {
				allMetric[sg][ver][1]  = allMetric[sg][ver][1] + 1
			}
		}
	}

	sendInf(allMetric,inf)
}

func sendInf(allMetric maps[string] map[string] [2]int, inf influxdb.client) {

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "prometheus",
	})

	t := time.Now()

	for k,_ := range allMetric {
		for kk,_ := range allMetric[k] {
			tags := map[string]string{"label_cluster": k, "version": kk}
			
			ddd := allMetric[k][kk]
			pt, err := client.NewPoint("sg_pod_success_num", tags, map[string]interface{}{"value": ddd[0]}, t)
			if err != nil {
				glog.Debugf("point fail:%v",err)
			}

			fmt.Printf("****pt***%+v\n",pt)
			bp.AddPoint(pt)

			pt, err = client.NewPoint("sg_pod_failed_num", tags, map[string]interface{}{"value": ddd[1]}, t)
			if err != nil {
				glog.Debugf("point fail:%v",err)
			}
			fmt.Printf("****pt***%+v\n",pt)
			bp.AddPoint(pt)
		}
	}
}


func main() {

	flag.Parse()

	redisUrl := *redisFlag
	if redisUrl == "" {
		glog.Infof("redis connection null")
		os.Exit(1)
	}

	client:=redis.NewClient(&redis.Options{
		Addr:redisUrl,
		Password:"",
		DB:0,
	})

	c, _ := client.NewHTTPClient(client.HTTPConfig{
		Addr:     *influxdbFlag,
		Username: "",
		Password: "",
	})

	fInterval := *interval

	FlushInterval := time.Duration( time.Duration(fInterval) * time.Second) 
	startCollect := time.Tick(FlushInterval)

	allSGInt := time.Duration(1 *  time.Hour)
	collectSG := time.Tick(allSGInt)
	fetchAll(client)
	FetchD(client,c)
	//loop:
	for {
		select {
		case <- startCollect: 
			FetchD(client)
		
		case <- collectSG: 
			fetchAll(client)
		}
	}
}
