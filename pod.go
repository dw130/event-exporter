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
	PODSTATUS = "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:instances:attributes:kubernetes:instances:%v"
	POCPRO = "kubernetes:instances:poc-qcloud-sh*"
	REG = regexp.MustCompile("\"podStatus\":[^\"]*\"([^\"]+)")
)

func FetchD(client *redis.Client) {
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
	fetchdetail(client)
}

func fetchAll(client *redis.Client) {
	mutex.Lock()
	defer mutex.Unlock()
}

func fetchdetail(client *redis.Client) {



	for k,_ := range allSG {
		pod := fmt.Sprintf(PODSTATUS, allSG[k][21:])
		val, err := client.Get(pod).Result()
		if err != nil {
			glog.Infof("fetch pod status fail:%v",err)
			continue
		}


		ret := REG.FindAllStringSubmatch(val,1)
		fmt.Printf("*****ret***%v\n",ret)
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

	fInterval := *interval

	FlushInterval := time.Duration( time.Duration(fInterval) * time.Second) 
	startCollect := time.Tick(FlushInterval)

	allSGInt := time.Duration(1 *  time.Hour)
	collectSG := time.Tick(allSGInt)
	fetchAll(client)
        FetchD(client)
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
