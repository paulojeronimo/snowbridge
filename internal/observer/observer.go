// PROPRIETARY AND CONFIDENTIAL
//
// Unauthorized copying of this file via any medium is strictly prohibited.
//
// Copyright (c) 2020-2021 Snowplow Analytics Ltd. All rights reserved.

package observer

import (
	log "github.com/sirupsen/logrus"
	"time"

	"github.com/snowplow-devops/stream-replicator/internal/models"
	"github.com/snowplow-devops/stream-replicator/internal/statsreceiver/statsreceiveriface"
)

// Observer holds the channels and settings for aggregating telemetry from processed messages
// and emitting them to downstream destinations
type Observer struct {
	statsClient              statsreceiveriface.StatsReceiver
	exitSignal               chan struct{}
	stopDone                 chan struct{}
	targetWriteChan          chan *models.TargetWriteResult
	targetWriteOversizedChan chan *models.TargetWriteResult
	timeout                  time.Duration
	reportInterval           time.Duration
	isRunning                bool

	log *log.Entry
}

// New builds a new observer to be used to gather telemetry
// about target writes
func New(statsClient statsreceiveriface.StatsReceiver, timeout time.Duration, reportInterval time.Duration) *Observer {
	return &Observer{
		statsClient:              statsClient,
		exitSignal:               make(chan struct{}),
		stopDone:                 make(chan struct{}),
		targetWriteChan:          make(chan *models.TargetWriteResult, 1000),
		targetWriteOversizedChan: make(chan *models.TargetWriteResult, 1000),
		timeout:                  timeout,
		reportInterval:           reportInterval,
		log:                      log.WithFields(log.Fields{"name": "Observer"}),
		isRunning:                false,
	}
}

// Start launches a goroutine which processes results from target writes
func (o *Observer) Start() {
	if o.isRunning {
		o.log.Warn("Observer is already running")
		return
	}
	o.isRunning = true

	go func() {
		reportTime := time.Now().Add(o.reportInterval)
		buffer := models.ObserverBuffer{}

	ObserverLoop:
		for {
			select {
			case <-o.exitSignal:
				o.log.Warn("Received exit signal, shutting down Observer ...")

				// Attempt final flush
				o.log.Infof(buffer.String())
				if o.statsClient != nil {
					o.statsClient.Send(&buffer)
				}

				o.isRunning = false
				break ObserverLoop
			case res := <-o.targetWriteChan:
				buffer.Append(res, false)
			case res := <-o.targetWriteOversizedChan:
				buffer.Append(res, true)
			case <-time.After(o.timeout):
				o.log.Debugf("Observer timed out after (%v) waiting for result", o.timeout)
			}

			if time.Now().After(reportTime) {
				o.log.Infof(buffer.String())
				if o.statsClient != nil {
					o.statsClient.Send(&buffer)
				}

				reportTime = time.Now().Add(o.reportInterval)
				buffer = models.ObserverBuffer{}
			}
		}
		o.stopDone <- struct{}{}
	}()
}

// Stop issues a signal to halt observer processing
func (o *Observer) Stop() {
	if o.isRunning {
		o.exitSignal <- struct{}{}
		<-o.stopDone
	}
}

// --- Functions called to push information to observer

// TargetWrite pushes a targets write result onto a channel for processing
// by the observer
func (o *Observer) TargetWrite(r *models.TargetWriteResult) {
	o.targetWriteChan <- r
}

// TargetWriteOversized pushes a failure targets write result onto a channel for processing
// by the observer
func (o *Observer) TargetWriteOversized(r *models.TargetWriteResult) {
	o.targetWriteOversizedChan <- r
}
