package redisc

import (
	"net"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestLoadInfo(t *testing.T) {
	g := NewGomegaWithT(t)

	reply := ""
	cn := &ClusterNode{}

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

	g.Expect(cn.Info.IP).Should(Equal("127.0.0.1"))
}
