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
	"time"
	"strings"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
        api_v1 "k8s.io/api/core/v1"
	//api_v1 "k8s.io/client-go/pkg/api/v1"
	influxdblib "github.com/influxdata/influxdb/client/v2"
)

var (
	receivedEntryCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "received_entry_count",
			Help:      "Number of entries, recieved by the Stackdriver sink",
			Subsystem: "stackdriver_sink",
		},
		[]string{"component"},
	)

	successfullySentEntryCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "successfully_sent_entry_count",
			Help:      "Number of entries, successfully ingested by Stackdriver",
			Subsystem: "stackdriver_sink",
		},
	)

	filter = map[string]int{
		"Pulled":0,
		"Pulling":0,
		"Starting":0,
		"Created":0,
		//"SandboxChanged":0,
		"Scheduled":0,
		"Started":0,
		"Killing":0,
	}
)

type sdSink struct {

	beforeFirstList    bool
	influxdbWriter     influxdbWriter
	pointChannel       chan *influxdblib.Point
	currentBufferInf   []*influxdblib.Point
}

func NewSink(influxdb string) *sdSink {
	w := newInfluxdbWriter(influxdb)
	return &sdSink{
		influxdbWriter:     w,
		beforeFirstList: true,
		pointChannel: make(chan *influxdblib.Point),
		currentBufferInf: []*influxdblib.Point{},
	}	
}


func (s *sdSink) OnAdd(event *api_v1.Event) {
	receivedEntryCount.WithLabelValues(event.Source.Component).Inc()

	oo := event.InvolvedObject
	if oo.Kind == "Pod" {
		name := oo.Name
		Reason := event.Reason

		_,ok := filter[Reason]
		if ok {
			return
		}
		if strings.Contains(Reason,"Success") {
			return
		}

		Message := event.Message
		clusterList := strings.Split(name,"-")
		cluster := strings.Join(clusterList[0: len(clusterList) - 2 ],"-")

		tags := map[string]string{"app_cluster": cluster}
		tags["pod_name"] = name
		//tags["container_name"] = 

		fields := map[string]interface{}{
			"reason": Reason,
			"message": Message,
		}

		p,_ := influxdblib.NewPoint("k8s-pod-event", tags, fields, time.Now())
		s.pointChannel <- p

		ss := strings.ToUpper(Message)
		if strings.Contains(ss,"OOM") {
			fields["value"] = 1
			p,_ := influxdblib.NewPoint("k8s-pod-oom", tags, fields, time.Now())
			s.pointChannel <- p			
		}

	}

}

func (s *sdSink) OnUpdate(oldEvent *api_v1.Event, newEvent *api_v1.Event) {
	var oldCount int32
	if oldEvent != nil {
		oldCount = oldEvent.Count
	}

	if newEvent.Count != oldCount+1 {
		// Sink doesn't send a LogEntry to Stackdriver, b/c event compression might
		// indicate that part of the watch history was lost, which may result in
		// multiple events being compressed. This may create an unecessary
		// flood in Stackdriver. Also this is a perfectly valid behavior for the
		// configuration with empty backing storage.
		glog.V(2).Infof("Event count has increased by %d != 1.\n"+
			"\tOld event: %+v\n\tNew event: %+v", newEvent.Count-oldCount, oldEvent, newEvent)
	}

	receivedEntryCount.WithLabelValues(newEvent.Source.Component).Inc()

	oo := newEvent.InvolvedObject
	if oo.Kind == "Pod" {
		name := oo.Name
		Reason := newEvent.Reason

		_,ok := filter[Reason]
		if ok {
			return
		}
		if strings.Contains(Reason,"Success") {
			return
		}

		Message := newEvent.Message
		clusterList := strings.Split(name,"-")
		cluster := strings.Join(clusterList[0: len(clusterList) - 2 ],"-")

		tags := map[string]string{"app_cluster": cluster}
		tags["pod_name"] = name
		//tags["container_name"] = 

		fields := map[string]interface{}{
			"reason": Reason,
			"message": Message,
		}

		p,_ := influxdblib.NewPoint("k8s-pod-event", tags, fields, time.Now())
		s.pointChannel <- p

		ss := strings.ToUpper(Message)
		if strings.Contains(ss,"OOM") {
			fields["value"] = 1
			p,_ := influxdblib.NewPoint("k8s-pod-oom", tags, fields, time.Now())
			s.pointChannel <- p			
		}
	}



}

func (s *sdSink) OnDelete(*api_v1.Event) {
	// Nothing to do here
}

func (s *sdSink) OnList(list *api_v1.EventList) {
	if s.beforeFirstList {
		//glog.Info("****entry OnList*******%v\n",list)
		s.beforeFirstList = false
	}
}

func (s *sdSink) Run(stopCh <-chan struct{}) {
	glog.Info("Starting Stackdriver sink")

	FlushInterval := 10 * time.Second
	startCollect := time.Tick(FlushInterval)

	loop:
	for {
		select {
		case entry := <-s.pointChannel:
			s.currentBufferInf = append(s.currentBufferInf, entry)
			if len(s.currentBufferInf) >= 100{
				s.flushBuffer()
			}
		case <- startCollect: 
			s.flushBuffer()
		case <-stopCh:
			glog.Info("Stackdriver sink recieved stop signal, waiting for all requests to finish")
			break loop
		}
	}
}

func (s *sdSink) flushBuffer() {
	s.sendEntries()
}

func (s *sdSink) sendEntries() {
	s.influxdbWriter.Write(s.currentBufferInf)
	s.currentBufferInf = []*influxdblib.Point{}
}
