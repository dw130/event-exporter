package main


import (
	"github.com/go-redis/redis"
	"fmt"
	"flag"
	"github.com/golang/glog"
	"os"
	"time"
	//"strings"
	"encoding/json"
	"sync"
	"regexp"
	influxdb "github.com/influxdata/influxdb/client/v2"
	"strings"
	"net/http"
	"io/ioutil"
)

var (
	redisFlag     = flag.String("redis", "", "redis")
	influxdbFlag  = flag.String("influxdb", "", "String")
	interval      = flag.Int("flush interval",40,"")
	env           = flag.String("env","poc","")
	gd            = flag.String("gd","","")
	gdInter       = flag.Int("gdtime",1800,"")
)

var (
	allSG = []string{}
	mutex   sync.Mutex
	mutex1  sync.Mutex
	allgb = []map[string]interface{}{}
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
	allSG = []string{}

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
	mutex1.Lock()
	defer mutex1.Unlock()
	//gd
	resp, err := http.Get(*gd)
	if err != nil {
		// handle error
		fmt.Printf("*****get meta data fail**%v\n",err)
		glog.Infof("%v",err)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Infof("%v",err)
		return
	}

	result := []map[string]interface{}{}

	json.Unmarshal(body, result) //解析json字符串

	allgb = result
}

func fetchdetail(client *redis.Client, inf  influxdb.Client) {

	allMetric := map[string] map[string] []int {}

	for k,_ := range allSG {

		all := strings.Split(allSG[k],":")
		var ver string
		var sg  string

		if len(all) != 0 {
			meta := all[ len(all) - 1 ]
			podList := strings.Split(meta,"-")
			ll := len(podList)
			if strings.Contains(allSG[k], "-v") == true && ll > 2 {
				//canarydemo-canary-v019-9dl9d
				//podId := podList[  ll - 1 ]
				ver   = podList[  ll - 2 ]
				sg    = strings.Join( podList[ 0: ll - 2 ], "-"  )
			} else {
				ver = "default"
				sg  =  strings.Join( podList[ 0: ll - 1 ],  "-" )
			}

			_,ok := allMetric[sg]
			if ok == false {
				allMetric[sg] = map[string] []int{}
			}
			_,ok = allMetric[sg][ver]
			if ok == false {
				allMetric[sg][ver] = []int{0,0}
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
		if len(ret) != 0 {
			pods := ret[0][1]
			if pods == "Running" {
				allMetric[sg][ver][0]  = allMetric[sg][ver][0] + 1
			} else {
				allMetric[sg][ver][1]  = allMetric[sg][ver][1] + 1
			}
		} else {
			allMetric[sg][ver][1]  = allMetric[sg][ver][1] + 1
		}
	}

	sendInf(allMetric,inf)
	sendInfGD(inf)
}

func sendInfGD(inf influxdb.Client) {
	mutex1.Lock()
	defer mutex1.Unlock()

	t := time.Now()

	bp, _ := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  "prometheus",
	})

	for k,_ := range allgb {
		d := allgb[k]
		//{"appName":"canarydemo","context":"poc","detail":null,"instanceNum":5,"namespace":"devops","stack":"main"}
		appname,ok := d["appName"]
		if ok == false {
			continue
		}

		stack,ok := d["stack"]
		if ok == false {
			continue
		}

		detail,ok := d["detail"]
		if ok == false {
			continue
		}

		var sg = ""
		if detail == nil {
			sg = fmt.Sprintf("%v-%v",appname,stack)
		} else {
			sg = fmt.Sprintf("%v-%v-%v",appname,stack, detail)
		}	

		tags := map[string]string{"label_cluster": sg}
			
		pt, err := influxdb.NewPoint("sg_gard_num", tags, map[string]interface{}{"value": d["instanceNum"]}, t)
		if err != nil {
			glog.Infof("point fail:%v",err)
		}

		fmt.Printf("***pt****%v\n",pt)
		bp.AddPoint(pt)

	}
	inf.Write(bp)
}

func sendInf(allMetric map[string] map[string] []int, inf influxdb.Client) {

	bp, _ := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  "prometheus",
	})

	t := time.Now()

	for k,_ := range allMetric {
		for kk,_ := range allMetric[k] {
			tags := map[string]string{"label_cluster": k, "version": kk}
			
			ddd := allMetric[k][kk]
			pt, err := influxdb.NewPoint("sg_pod_success_num", tags, map[string]interface{}{"value": ddd[0]}, t)
			if err != nil {
				glog.Infof("point fail:%v",err)
			}

			//fmt.Printf("****pt***%+v\n",pt)
			bp.AddPoint(pt)

			pt, err = influxdb.NewPoint("sg_pod_failed_num", tags, map[string]interface{}{"value": ddd[1]}, t)
			if err != nil {
				glog.Infof("point fail:%v",err)
			}
			//fmt.Printf("****pt***%+v\n",pt)
			bp.AddPoint(pt)
		}
	}
	inf.Write(bp)
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

	c, _ := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     *influxdbFlag,
		Username: "",
		Password: "",
	})

	fInterval := *interval

	FlushInterval := time.Duration( time.Duration(fInterval) * time.Second) 

	FlushInterval1 := time.Duration( time.Duration(*gdInter) * time.Second)

	startCollect1 := time.Tick(FlushInterval1)

	collectSG := time.Tick(FlushInterval)

	fetchAll(client)
	FetchD(client,c)
	//loop:
	for {
		select {
		case <- collectSG: 
			FetchD(client,c)
		
		case <- startCollect1: 
			fetchAll(client)
		}
	}
}
