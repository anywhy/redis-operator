package redis

import (
	"context"
	"time"
)

const (
	timeout = 30 * time.Second
)

// Sentinel redis sentinel client
type Sentinel struct {
	context.Context
	Cancel context.CancelFunc

	MasterName, Addr, Auth string

	Client *Client
}

// NewSentinel return a redis sentinel
func NewSentinel(masterName, addr, auth string) (*Sentinel, error) {
	var err error
	s := &Sentinel{MasterName: masterName, Addr: addr, Auth: auth}
	s.Context, s.Cancel = context.WithCancel(context.Background())
	s.Client, err = NewClient(addr, auth, timeout)
	return s, err
}
