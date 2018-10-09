/*
Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package stackdriver

import (
	"github.com/golang/glog"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	influxdblib "github.com/influxdata/influxdb/client/v2"
)

const (
	retryDelay = 10 * time.Second
)

var (
	requestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "request_count",
			Help:      "Number of request, issued to Stackdriver API",
			Subsystem: "stackdriver_sink",
		},
		[]string{"code"},
	)
)



type influxdbWriter interface {
	Write([]*influxdblib.Point)
}

type influxdbWriterImpl struct {
	service influxdblib.Client
}

func newInfluxdbWriter(influxdb string) influxdbWriter {

	conn, _ := influxdblib.NewHTTPClient(influxdblib.HTTPConfig{
		Addr:     influxdb,
		Username: "",
		Password: "",
	})

	return &influxdbWriterImpl{
		service: conn,
	}
}

func (w *influxdbWriterImpl) Write(ps  []*influxdblib.Point)  {

	bp, err := influxdblib.NewBatchPoints(influxdblib.BatchPointsConfig{
		Database:  "prometheus",
	})

	for k,_ := range ps {
		//glog.Info("***write***Point*****%+v\n",ps[k])
		bp.AddPoint(ps[k])
	}

	err = w.service.Write(bp) 
	if err != nil {
		glog.Info(err)
	}
}


