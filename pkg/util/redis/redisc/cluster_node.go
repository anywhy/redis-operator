package redisc

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anywhy/redis-operator/pkg/util/redis"
	redigo "github.com/gomodule/redigo/redis"
)

const (
	connTimeout = time.Second * 60

	// UnusedHashSlot unused slot flag
	UnusedHashSlot = iota
	// NewHashSlot new solt flag
	NewHashSlot
	// AssignedHashSlot assigned slot flag
	AssignedHashSlot
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

// LoadInfo get cluster info from current node
func (cn *ClusterNode) LoadInfo() error {
	reply, err := redigo.String(cn.Call("CLUSTER", "NODES"))
	if err != nil {
		return err
	}

	nodes := strings.Split(reply, "\n")
	for _, node := range nodes {
		parts := strings.Split(node, " ")
		if len(parts) <= 3 {
			continue
		}

		sent, _ := strconv.ParseInt(parts[4], 0, 32)
		recv, _ := strconv.ParseInt(parts[5], 0, 32)
		addr := strings.Split(parts[1], "@")[0]
		host, port, _ := net.SplitHostPort(addr)
		p, _ := strconv.ParseUint(port, 10, 0)
		node := &Node{
			Name:       parts[0],
			Flags:      strings.Split(parts[2], ","),
			Replicate:  parts[3],
			PingSent:   int(sent),
			PingRecv:   int(recv),
			LinkStatus: parts[6],

			IP:        host,
			Port:      int(p),
			Slots:     make(map[int]int),
			Migrating: make(map[int]string),
			Importing: make(map[int]string),
		}

		if parts[3] == "-" {
			node.Replicate = ""
		}

		if strings.Contains(parts[2], "myself") {
			if cn.Info != nil {
				cn.Info.Name = node.Name
				cn.Info.Flags = node.Flags
				cn.Info.Replicate = node.Replicate
				cn.Info.PingSent = node.PingSent
				cn.Info.PingRecv = node.PingRecv
				cn.Info.LinkStatus = node.LinkStatus
			} else {
				cn.Info = node
			}

			for i := 8; i < len(parts); i++ {
				slots := parts[i]
				if strings.Contains(slots, "<") {
					slotStr := strings.Split(slots, "-<-")
					slotID, _ := strconv.Atoi(slotStr[0])
					cn.Info.Importing[slotID] = slotStr[1]
				} else if strings.Contains(slots, ">") {
					slotStr := strings.Split(slots, "->-")
					slotID, _ := strconv.Atoi(slotStr[0])
					cn.Info.Migrating[slotID] = slotStr[1]
				} else if strings.Contains(slots, "-") {
					slotStr := strings.Split(slots, "-")
					firstID, _ := strconv.Atoi(slotStr[0])
					lastID, _ := strconv.Atoi(slotStr[1])
					cn.AddSlots(firstID, lastID)
				} else {
					firstID, _ := strconv.Atoi(slots)
					cn.AddSlots(firstID, firstID)
				}
			}
		} else {
			cn.Friends = append(cn.Friends, node)
		}
	}

	return nil
}

// FlushNodeConfig flush node config
func (cn *ClusterNode) FlushNodeConfig() {
	if !cn.Dirty {
		return
	}

	if cn.Info.Replicate != "" {
		if _, err := cn.ClusterReplicateWithNodeID(cn.Info.Replicate); err != nil {
			// If the cluster did not already joined it is possible that
			// the slave does not know the master node yet. So on errors
			// we return ASAP leaving the dirty flag set, to flush the
			// config later.
			return
		}
	} else {
		var array []int
		for s, value := range cn.Info.Slots {
			if value == NewHashSlot {
				array = append(array, s)
				cn.Info.Slots[s] = AssignedHashSlot
			}
			cn.ClusterAddSlots(array)
		}
	}

	cn.Dirty = false
}

// AddSlots add slot to cluster node
func (cn *ClusterNode) AddSlots(start, end int) {
	for i := start; i <= end; i++ {
		cn.Info.Slots[i] = NewHashSlot
	}
	cn.Dirty = true
}

// GetConfigSignature get cluster node config signature
func (cn *ClusterNode) GetConfigSignature() string {
	configs := []string{}

	reply, err := redigo.String(cn.Call("CLUSTER", "NODES"))
	if err != nil {
		return ""
	}

	nodes := strings.Split(reply, "\n")
	for _, node := range nodes {
		parts := strings.Split(node, " ")
		if len(parts) <= 3 {
			continue
		}

		sig := parts[0] + ":"
		slots := []string{}
		if len(parts) > 7 {
			for i := 8; i < len(parts); i++ {
				p := parts[i]
				if !strings.Contains(p, "[") {
					slots = append(slots, p)
				}
			}
		}
		if len(slots) == 0 {
			continue
		}
		sort.Strings(slots)
		sig = sig + strings.Join(slots, ",")

		configs = append(configs, sig)
	}

	return strings.Join(configs, "|")
}
