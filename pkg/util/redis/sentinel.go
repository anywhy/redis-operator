package redis

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	redigo "github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"

	"github.com/anywhy/redis-operator/pkg/util/sync2/atomic2"
)

// Sentinel redis sentinel client
type Sentinel struct {
	context.Context
	Cancel context.CancelFunc

	MasterName, Auth string
}

// NewSentinel return a redis sentinel
func NewSentinel(masterName, auth string) *Sentinel {
	s := &Sentinel{MasterName: masterName, Auth: auth}
	s.Context, s.Cancel = context.WithCancel(context.Background())
	return s
}

// IsCanceled sentinel is canceled
func (s *Sentinel) IsCanceled() bool {
	select {
	case <-s.Context.Done():
		return true
	default:
		return false
	}
}

// Subscribe subscribe sentinel
func (s *Sentinel) Subscribe(sentinels []string, timeout time.Duration, onMajoritySubscribed func()) bool {
	cntx, cancel := context.WithTimeout(s.Context, timeout)
	defer cancel()

	timeout += time.Second * 5
	results := make(chan bool, len(sentinels))
	var majority = 1 + len(sentinels)/2

	var subscribed atomic2.Int64
	for i := range sentinels {
		go func(sentinel string) {
			notified, err := s.subscribeDispatch(cntx, sentinel, timeout, func() {
				if subscribed.Incr() == int64(majority) {
					onMajoritySubscribed()
				}
			})
			if err != nil {
				glog.Errorf("sentinel-[%s] subscribe failed", sentinel)
			}
			results <- notified
		}(sentinels[i])
	}

	for alive := len(sentinels); ; alive-- {
		if alive < majority {
			if cntx.Err() == nil {
				glog.Errorf("sentinel subscribe lost majority (%d/%d)", alive, len(sentinels))
			}
			return false
		}
		select {
		case <-cntx.Done():
			if cntx.Err() != context.DeadlineExceeded {
				glog.Errorf("sentinel subscribe canceled (%v)", cntx.Err())
			}
			return false
		case notified := <-results:
			if notified {
				glog.Infof("sentinel subscribe notified +switch-master")
				return true
			}
		}
	}
}

func (s *Sentinel) subscribeDispatch(ctx context.Context, sentinel string, timeout time.Duration,
	onSubscribed func()) (bool, error) {
	var err = s.dispatch(ctx, sentinel, timeout, func(c *Client) error {
		return s.subscribeCommand(c, sentinel, onSubscribed)
	})
	if err != nil {
		switch err {
		case context.Canceled, context.DeadlineExceeded:
			return false, nil
		default:
			return false, err
		}
	}
	return true, nil
}

func (s *Sentinel) dispatch(ctx context.Context, sentinel string, timeout time.Duration,
	fn func(client *Client) error) error {
	c, err := NewClient(sentinel, s.Auth, timeout)
	if err != nil {
		return err
	}
	defer c.Close()

	var exit = make(chan error, 1)

	go func() {
		exit <- fn(c)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-exit:
		return err
	}
}

func (s *Sentinel) subscribeCommand(client *Client, sentinel string,
	onSubscribed func()) error {
	defer func() {
		client.Close()
	}()
	var channels = []interface{}{"+switch-master"}
	go func() {
		client.Send("SUBSCRIBE", channels...)
		client.Flush()
	}()
	for _, sub := range channels {
		values, err := redigo.Values(client.Receive())
		if err != nil {
			return err
		} else if len(values) != 3 {
			return fmt.Errorf("invalid response = %v", values)
		}
		s, err := redigo.Strings(values[:2], nil)
		if err != nil || s[0] != "subscribe" || s[1] != sub.(string) {
			return fmt.Errorf("invalid response = %v", values)
		}
	}
	onSubscribed()
	for {
		values, err := redigo.Values(client.Receive())
		if err != nil {
			return err
		} else if len(values) < 2 {
			return fmt.Errorf("invalid response = %v", values)
		}
		message, err := redigo.Strings(values, nil)
		if err != nil || message[0] != "message" {
			return fmt.Errorf("invalid response = %v", values)
		}
		glog.Infof("sentinel-[%s] subscribe event %v", sentinel, message)

		switch message[1] {
		case "+switch-master":
			if len(message) != 3 {
				return fmt.Errorf("invalid response = %v", values)
			}
			var params = strings.SplitN(message[2], " ", 2)
			if len(params) != 2 {
				return fmt.Errorf("invalid response = %v", values)
			}
			if strings.EqualFold(s.MasterName, params[2]) {
				return nil
			}
		}
	}
}

func (s *Sentinel) masterCommand(client *Client) (map[string]string, error) {
	defer func() {
		if !client.isRecyclable() {
			client.Close()
		}
	}()

	return redigo.StringMap(client.Do("SENTINEL", "master", s.MasterName))
}

// SentinelMaster sentinel master
type SentinelMaster struct {
	Addr  string
	Info  map[string]string
	Epoch int64
}

func (s *Sentinel) masterDispatch(ctx context.Context, sentinel string, timeout time.Duration) (*SentinelMaster, error) {
	var master *SentinelMaster
	var err = s.dispatch(ctx, sentinel, timeout, func(c *Client) error {
		infos, err := s.masterCommand(c)
		if err != nil {
			return err
		}

		epoch, err := strconv.ParseInt(infos["config-epoch"], 10, 64)
		if err != nil {
			glog.Warningf("sentinel-[%s] masters parse %s failed, config-epoch = '%s', %s",
				sentinel, infos["name"], infos["config-epoch"], err)
			return nil
		}
		var ip, port = infos["ip"], infos["port"]
		if ip == "" || port == "" {
			glog.Warningf("sentinel-[%s] masters parse %s failed, ip:port = '%s:%s'",
				sentinel, infos["name"], ip, port)
			return nil
		}
		master = &SentinelMaster{
			Addr:  net.JoinHostPort(ip, port),
			Info:  infos,
			Epoch: epoch,
		}

		return nil
	})
	if err != nil {
		switch errors.Cause(err) {
		case context.Canceled:
			return nil, nil
		default:
			return nil, err
		}
	}
	return master, nil
}

// Master get sentinel monitor master address
func (s *Sentinel) Master(sentinels []string, timeout time.Duration) (string, error) {
	cntx, cancel := context.WithTimeout(s.Context, timeout)
	defer cancel()

	timeout += time.Second * 5
	results := make(chan *SentinelMaster, len(sentinels))

	var majority = 1 + len(sentinels)/2

	for i := range sentinels {
		go func(sentinel string) {
			master, err := s.masterDispatch(cntx, sentinel, timeout)
			if err != nil {
				glog.Errorf("sentinel-[%s] master failed", sentinel)
			}
			results <- master
		}(sentinels[i])
	}

	master := ""
	var current *SentinelMaster

	var voted int
	for alive := len(sentinels); ; alive-- {
		if alive == 0 {
			switch {
			case cntx.Err() != context.DeadlineExceeded && cntx.Err() != nil:
				glog.Errorf("sentinel master canceled (%v)", cntx.Err())
				return "", cntx.Err()
			case voted != len(sentinels):
				glog.Errorf("sentinel master voted = (%d/%d) master = %s (%v)", voted, len(sentinels), master, cntx.Err())
			}
			if voted < majority {
				return "", fmt.Errorf("lost majority (%d/%d)", voted, len(sentinels))
			}
			return master, nil
		}
		select {
		case <-cntx.Done():
			switch {
			case cntx.Err() != context.DeadlineExceeded:
				glog.Errorf("sentinel masters canceled (%v)", cntx.Err())
				return "", cntx.Err()
			default:
				glog.Infof("sentinel masters voted = (%d/%d) masters = %s (%v)", voted, len(sentinels), master, cntx.Err())
			}
			if voted < majority {
				return "", fmt.Errorf("lost majority (%d/%d)", voted, len(sentinels))
			}
			return master, nil
		case m := <-results:
			if m == nil {
				continue
			}
			if current == nil || current.Epoch < m.Epoch {
				current = m
				master = m.Addr
			}
			voted++
		}
	}
}
