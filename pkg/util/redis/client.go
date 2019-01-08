package redis

import (
	"time"

	"github.com/anywhy/redis-operator/pkg/util/math2"
	redigo "github.com/gomodule/redigo/redis"
)

// Client redis client
type Client struct {
	conn redigo.Conn
	Addr string
	Auth string

	Database int

	LastUse time.Time
	Timeout time.Duration

	Pipeline struct {
		Send, Recv uint64
	}
}

// NewClientNoAuth return a redis client has not auth
func NewClientNoAuth(addr string, timeout time.Duration) (*Client, error) {
	return NewClient(addr, "", timeout)
}

// NewClient return a redis client
func NewClient(addr string, auth string, timeout time.Duration) (*Client, error) {
	c, err := redigo.Dial("tcp", addr, []redigo.DialOption{
		redigo.DialConnectTimeout(math2.MinDuration(time.Second, timeout)),
		redigo.DialPassword(auth),
		redigo.DialReadTimeout(timeout), redigo.DialWriteTimeout(timeout),
	}...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: c, Addr: addr, Auth: auth,
		LastUse: time.Now(), Timeout: timeout,
	}, nil
}

// Close close a redis connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Do execute a redis command
func (c *Client) Do(cmd string, args ...interface{}) (interface{}, error) {
	r, err := c.conn.Do(cmd, args...)
	if err != nil {
		c.Close()
		return nil, err
	}
	c.LastUse = time.Now()

	if err, ok := r.(redigo.Error); ok {
		return nil, err
	}
	return r, nil
}

// Send send a command to redis server
func (c *Client) Send(cmd string, args ...interface{}) error {
	if err := c.conn.Send(cmd, args...); err != nil {
		c.Close()
		return err
	}
	c.Pipeline.Send++
	return nil
}

// Flush flush command
func (c *Client) Flush() error {
	if err := c.conn.Flush(); err != nil {
		c.Close()
		return err
	}
	return nil
}

// Receive receive batch command returns
func (c *Client) Receive() (interface{}, error) {
	r, err := c.conn.Receive()
	if err != nil {
		c.Close()
		return nil, err
	}
	c.Pipeline.Recv++

	c.LastUse = time.Now()

	if err, ok := r.(redigo.Error); ok {
		return nil, err
	}
	return r, nil
}

func (c *Client) isRecyclable() bool {
	switch {
	case c.conn.Err() != nil:
		return false
	case c.Pipeline.Send != c.Pipeline.Recv:
		return false
	case c.Timeout != 0 && c.Timeout <= time.Since(c.LastUse):
		return false
	}
	return true
}
