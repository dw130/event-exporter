package main


import (
	"github.com/go-redis/redis"
	"fmt"
	"flag"
	"github.com/golang/glog"
	"os"
	"time"
	"strings"
	"encoding/json"
	"sync"
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
	POCPRO = "kubernetes:instances:poc-qcloud-sh"
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
	    keys, cursor, err = client.Scan(INSTANCE, cursor, prefix, 100).Result()
	    if err != nil {
	        glog.Infof("redis scan err:%v",err)
	        break loop
	    }
	    count += len(keys)
	    if cursor == 1 {
	        break loop
	    }
	    allSG = append(allSG,keys..)
	}
	glog.Infof("fetch total sg number:%v",count)
	fetchdetail(client)
}

func fetchAll(client *redis.Client,pod string) {
	mutex.Lock()
	defer mutex.Unlock()
}

func fetchdetail(client *redis.Client) {
	for k,_ := range allSG {
		pod := fmt.Sprintf(PODSTATUS, allSG[k][21:])
		val, err := client.Get("key").Result()
		if err != nil {
			glog.Infof("fetch pod status fail:%v",err)
			continue
		}
		var ret map[string] interface{}

		err := json. Unmarshal ( val , &ret ) 
	    if err != nil { 
	        fmt. Println ( "error:" , err ) 
	    }
	    fmt.Printf("fetchdetail:%v",ret)
	}
}



func fetchData(client *redis.Client) {
	n := time.Now()

	pipe := client.Pipeline()

	var cursor uint64
	var count int

	loop:
	for {
	    var keys []string
	    var err error
	    keys, cursor, err = client.Scan(cursor, "com.netflix.spinnaker.clouddriver.kubernetes.v1.provider.KubernetesV1Provider:clusters:attributes:kubernetes:clusters:*:serverGroup:*", 100).Result()
	    if err != nil {
	        glog.Infof("redis scan err:%v",err)
	        break loop
	    }
	    count += len(keys)
	    if cursor == 0 {
	        break
	    }
	    fetchdetail(client,keys)
	}

	glog.Infof("total serverGroup num:%v",count)
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

	FlushInterval := time.Duration(fInterval * time.Second )
	startCollect := time.Tick(FlushInterval)

	allSGInt := time.Duration(1 *  time.Hour)
	collectSG := time.Tick(allSGInt)

	fetchAll(client)
	loop:
	for {
		select {
		case <- startCollect: 
			FetchD(client)
		}
		case <- collectSG: 
			fetchAll(client)
		}
	}
}
