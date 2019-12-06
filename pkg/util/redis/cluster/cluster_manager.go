package redis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/utils/exec"
)

// ClusterManager redis cluster manager use by redis-cli
type ClusterManager struct {
	Context context.Context

	cmder exec.Interface

	auth  string
	nodes []string
}

// NewClusterManager create a cluster manager
func NewClusterManager(nodes []string, auth string) *ClusterManager {
	return &ClusterManager{
		nodes:   nodes,
		auth:    auth,
		cmder:   exec.New(),
		Context: context.Background(),
	}
}

// CreateCluster create a cluster
func (cm *ClusterManager) CreateCluster(replicas int, timemout time.Duration) error {
	// create cluster
	ctx, cancle := context.WithTimeout(cm.Context, timemout)
	defer cancle()

	params := append([]string{"create"}, cm.nodes...)
	cmd := cm.commandContext(ctx, append(params, "--cluster-replicas", fmt.Sprintf("%d", replicas))...)
	cmd.SetStdin(bytes.NewBufferString("yes"))

	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "Either the node already knows other nodes") {
		if len(out) == 0 {
			out = []byte(err.Error())
		}
		return errors.New(string(out))
	}

	glog.Info(string(out))
	return nil
}

// AddNode add node to cluster
func (cm *ClusterManager) AddNode(addr string, port int) error {
	out, err := cm.command(append([]string{"add-node"},
		fmt.Sprintf("%s:%d", addr, port), cm.nodes[0])...).CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}
		return errors.New(string(out))
	}

	cm.nodes = append(cm.nodes, fmt.Sprintf("%s:%d", addr, port))
	glog.Info(string(out))
	return nil
}

//Command create a Command
func (cm *ClusterManager) command(args ...string) exec.Cmd {
	params := []string{"--cluster"}
	return cm.cmder.Command("redis-cli", append(params, args...)...)
}

//CommandContext create a CommandContext
func (cm *ClusterManager) commandContext(ctx context.Context, args ...string) exec.Cmd {
	params := []string{"--cluster"}
	return cm.cmder.CommandContext(ctx, "redis-cli", append(params, args...)...)
}
