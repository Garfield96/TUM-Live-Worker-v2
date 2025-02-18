package worker

import (
	"context"
	"fmt"
	"github.com/joschahenningsen/TUM-Live-Worker-v2/cfg"
	"github.com/joschahenningsen/TUM-Live-Worker-v2/pb"
	"github.com/joschahenningsen/TUM-Live-Worker-v2/worker/vmstat"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"strings"
	"sync"
	"time"
)

var statusLock = sync.RWMutex{}
var S *Status
var VersionTag string

func init() {
	stat := vmstat.New()
	S = &Status{
		workload:  0,
		StartTime: time.Now(),
		Jobs:      []string{},
		Stat:      stat,
	}
	c := cron.New()
	_, _ = c.AddFunc("* * * * *", S.SendHeartbeat)
	_, _ = c.AddFunc("* * * * *", func() {
		err := S.Stat.Update()
		if err != nil {
			log.WithError(err).Warn("Failed to update vmstat")
		}
	})
	c.Start()
}

const (
	costStream           = 3
	costTranscoding      = 2
	costSilenceDetection = 1
)

type Status struct {
	workload  uint
	Jobs      []string
	StartTime time.Time

	// VM Metrics are updated regularly
	Stat *vmstat.VmStat
}

func (s *Status) startSilenceDetection(streamCtx *StreamContext) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	s.workload += costSilenceDetection
	s.Jobs = append(s.Jobs, fmt.Sprintf("detecting silence in %s", streamCtx.getStreamName()))
	statusLock.Unlock()
}

func (s *Status) startStream(streamCtx *StreamContext) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	notifyStreamStart(streamCtx)
	defer statusLock.Unlock()
	s.workload += costStream
	s.Jobs = append(s.Jobs, fmt.Sprintf("streaming %s", streamCtx.getStreamName()))
}

func (s *Status) startRecording(name string) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	defer statusLock.Unlock()
	s.workload += costStream
	s.Jobs = append(s.Jobs, fmt.Sprintf("recording %s", name))
}

func (s *Status) startTranscoding(name string) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	defer statusLock.Unlock()
	s.workload += costTranscoding
	s.Jobs = append(s.Jobs, fmt.Sprintf("transcoding %s", name))
}

func (s *Status) endStream(streamCtx *StreamContext) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	s.workload -= costStream
	for i := range s.Jobs {
		if s.Jobs[i] == fmt.Sprintf("streaming %s", streamCtx.getStreamName()) {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			break
		}
	}
	statusLock.Unlock()
}

func (s *Status) endRecording(name string) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	s.workload -= costStream
	for i := range s.Jobs {
		if s.Jobs[i] == fmt.Sprintf("recording %s", name) {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			break
		}
	}
	statusLock.Unlock()
}

func (s *Status) endTranscoding(name string) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	s.workload -= costTranscoding
	for i := range s.Jobs {
		if s.Jobs[i] == fmt.Sprintf("transcoding %s", name) {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			break
		}
	}
	statusLock.Unlock()
}

func (s *Status) endSilenceDetection(streamCtx *StreamContext) {
	defer s.SendHeartbeat()
	statusLock.Lock()
	s.workload -= costSilenceDetection
	for i := range s.Jobs {
		if s.Jobs[i] == fmt.Sprintf("detecting silence in %s", streamCtx.getStreamName()) {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			break
		}
	}
	statusLock.Unlock()
}

func (s *Status) SendHeartbeat() {
	// WithInsecure: workerId used for authentication, all servers are inside their own VLAN to further improve security
	clientConn, err := grpc.Dial(fmt.Sprintf("%s:50052", cfg.MainBase), grpc.WithInsecure())
	if err != nil {
		log.WithError(err).Error("unable to dial for heartbeat")
		return
	}
	client := pb.NewFromWorkerClient(clientConn)
	defer func(clientConn *grpc.ClientConn) {
		err := clientConn.Close()
		if err != nil {
			log.WithError(err).Warn("Can't close status req")
		}
	}(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	_, err = client.SendHeartBeat(ctx, &pb.HeartBeat{
		WorkerID: cfg.WorkerID,
		Workload: uint32(s.workload),
		Jobs:     s.Jobs,
		Version:  VersionTag,
		CPU:      s.Stat.GetCpuStr(),
		Memory:   s.Stat.GetMemStr(),
		Disk:     s.Stat.GetDiskStr(),
		Uptime:   strings.ReplaceAll(time.Since(s.StartTime).Round(time.Minute).String(), "0s", ""),
	})
	if err != nil {
		log.WithError(err).Error("Sending Heartbeat failed")
	}
}
