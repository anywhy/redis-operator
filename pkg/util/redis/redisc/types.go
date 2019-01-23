package redisc

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/anywhy/redis-operator/pkg/util/redis"
)

const (
	connTimeout = time.Second * 60
)

// Node redis instance attributes
type Node struct {
	Name       string
	IP         string
	Port       int
	Flags      []string
	Replicate  string
	PingSent   int
	PingRecv   int
	Weight     int
	Balance    int
	LinkStatus string
	Slots      map[int]int
	Migrating  map[int]string
	Importing  map[int]string
}

// ClusterNode redis cluster node
type ClusterNode struct {
	client *redis.Client

	Info          *Node
	Dirty         bool
	Friends       [](*Node)
	ReplicasNodes [](*ClusterNode)
}

// NewClusterNode return a new cluster node
func NewClusterNode(addr string) (*ClusterNode, error) {
	parts := strings.Split(addr, ":")
	p, _ := strconv.ParseUint(parts[1], 10, 0)
	node := &ClusterNode{
		Info: &Node{
			IP:        parts[0],
			Port:      int(p),
			Slots:     make(map[int]int),
			Migrating: make(map[int]string),
			Importing: make(map[int]string),
			Replicate: "",
		},
		Dirty: false,
	}
	client, err := redis.NewClientNoAuth(addr, connTimeout)
	if err != nil {
		return nil, err
	}
	node.client = client
	return node, nil
}

// Addr cluster node addr
func (cn *ClusterNode) Addr() string {
	return fmt.Sprintf("%s:%s", cn.Info.IP, strconv.Itoa(cn.Info.Port))
}

// HasFlag has flag
func (cn *ClusterNode) HasFlag(flag string) bool {
	for _, f := range cn.Info.Flags {
		if strings.Contains(f, flag) {
			return true
		}
	}
	return false
}

// Call call client do somethings
func (cn *ClusterNode) Call(cmd string, args ...interface{}) (interface{}, error) {
	return cn.client.Do(cmd, args...)
}

// ClusterAddNode add node to cluster
func (cn *ClusterNode) ClusterAddNode(addr string) (ret string, err error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || port == "" {
		return "", fmt.Errorf("Bad format of host:port: %s", addr)
	}
	return redigo.String(cn.Call("CLUSTER", "MEET", host, port))
}

// ClusterReplicateWithNodeID add replica node
func (cn *ClusterNode) ClusterReplicateWithNodeID(nodeid string) (ret string, err error) {
	return redigo.String(cn.Call("CLUSTER", "REPLICATE", nodeid))
}

// ClusterForgetNodeID remove a node from cluster
func (cn *ClusterNode) ClusterForgetNodeID(nodeid string) (ret string, err error) {
	return redigo.String(cn.Call("CLUSTER", "FORGET", nodeid))
}

// ClusterCountKeysInSlot clount key in slot
func (cn *ClusterNode) ClusterCountKeysInSlot(slot int) (int, error) {
	return redigo.Int(cn.Call("CLUSTER", "COUNTKEYSINSLOT", slot))
}

// ClusterGetKeysInSlot get key form slot
func (cn *ClusterNode) ClusterGetKeysInSlot(slot int, pipeline int) (string, error) {
	return redigo.String(cn.Call("CLUSTER", "GETKEYSINSLOT", slot, pipeline))
}

// ClusterSetSlot set cluster slot
func (cn *ClusterNode) ClusterSetSlot(slot int, cmd string) (string, error) {
	return redigo.String(cn.Call("CLUSTER", "SETSLOT", slot, cmd, cn.Info.Name))
}

// ClusterAddSlots add slot to cluster node
func (cn *ClusterNode) ClusterAddSlots(args ...interface{}) (ret string, err error) {
	return redigo.String(cn.Call("CLUSTER", "ADDSLOTS", args))
}

// ClusterDelSlots delete slot from cluster node
func (cn *ClusterNode) ClusterDelSlots(args ...interface{}) (ret string, err error) {
	return redigo.String(cn.Call("CLUSTER", "DELSLOTS", args))
}
