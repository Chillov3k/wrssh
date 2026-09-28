package data

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NetworkNode is a host discovered by pscan in this project's network. It only
// stores what scanning learned (open ports, NetBIOS hints); live agent session
// state is merged on top by the control plane.
type NetworkNode struct {
	ID          uint       `json:"id"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	IP          string     `gorm:"uniqueIndex;size:64" json:"ip"`
	Hostname    string     `gorm:"size:255" json:"hostname"`
	OS          string     `gorm:"size:32" json:"os"`
	Ports       string     `gorm:"size:2048" json:"ports"`
	FirstSeenAt *time.Time `json:"firstSeenAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
}

// NetworkEdge records that one node was reachable from a pscan run started on
// another node.
type NetworkEdge struct {
	ID          uint       `json:"id"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	FromIP      string     `gorm:"uniqueIndex:idx_network_edges_endpoints,priority:1;size:64" json:"fromIp"`
	ToIP        string     `gorm:"uniqueIndex:idx_network_edges_endpoints,priority:2;size:64" json:"toIp"`
	FirstSeenAt *time.Time `json:"firstSeenAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
}

// NetworkScanTarget is one discovered host from a single pscan run.
type NetworkScanTarget struct {
	IP       string
	Hostname string
	OS       string
	Ports    []string // "22/tcp", "53/udp"
}

// UpsertNetworkScan records one finished pscan run: every discovered host and
// a directed edge from the scanning host to each of them.
func UpsertNetworkScan(sourceIP string, targets []NetworkScanTarget) error {
	sourceIP = strings.TrimSpace(sourceIP)
	if sourceIP == "" {
		return errors.New("scan source ip is required")
	}
	if len(targets) == 0 {
		return nil
	}

	db := DB()
	if db == nil {
		return errors.New("database is not loaded")
	}

	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, target := range targets {
			ip := strings.TrimSpace(target.IP)
			if ip == "" {
				continue
			}

			var node NetworkNode
			err := tx.Where("ip = ?", ip).Limit(1).Find(&node).Error
			if err != nil {
				return err
			}

			ports := mergeNetworkPorts(node.Ports, target.Ports)
			if node.IP == "" {
				node = NetworkNode{
					IP:          ip,
					Hostname:    strings.TrimSpace(target.Hostname),
					OS:          strings.TrimSpace(target.OS),
					Ports:       ports,
					FirstSeenAt: ptrTime(now),
					LastSeenAt:  ptrTime(now),
				}
				if err := tx.Create(&node).Error; err != nil {
					return err
				}
			} else {
				updates := map[string]any{
					"ports":        ports,
					"last_seen_at": now,
				}
				if strings.TrimSpace(node.Hostname) == "" && strings.TrimSpace(target.Hostname) != "" {
					updates["hostname"] = strings.TrimSpace(target.Hostname)
				}
				if strings.TrimSpace(node.OS) == "" && strings.TrimSpace(target.OS) != "" {
					updates["os"] = strings.TrimSpace(target.OS)
				}
				if err := tx.Model(&NetworkNode{}).Where("ip = ?", ip).Updates(updates).Error; err != nil {
					return err
				}
			}

			if ip == sourceIP {
				continue
			}

			edge := NetworkEdge{
				FromIP:      sourceIP,
				ToIP:        ip,
				FirstSeenAt: ptrTime(now),
				LastSeenAt:  ptrTime(now),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "from_ip"}, {Name: "to_ip"}},
				DoUpdates: clause.AssignmentColumns([]string{"last_seen_at", "updated_at"}),
			}).Create(&edge).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ListNetworkNodes returns every discovered node ordered by IP.
func ListNetworkNodes() ([]NetworkNode, error) {
	var nodes []NetworkNode
	err := DB().Order("ip asc").Find(&nodes).Error
	return nodes, err
}

// ListNetworkEdges returns every discovered edge ordered by endpoints.
func ListNetworkEdges() ([]NetworkEdge, error) {
	var edges []NetworkEdge
	err := DB().Order("from_ip asc, to_ip asc").Find(&edges).Error
	return edges, err
}

// ClearNetworkData removes all stored scan discoveries for this runtime.
func ClearNetworkData() error {
	return DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&NetworkEdge{}).Error; err != nil {
			return err
		}
		return tx.Where("1 = 1").Delete(&NetworkNode{}).Error
	})
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func mergeNetworkPorts(existing string, incoming []string) string {
	seen := map[string]struct{}{}
	ports := make([]string, 0, len(incoming))
	appendPort := func(port string) {
		port = strings.TrimSpace(port)
		if port == "" {
			return
		}
		if _, ok := seen[port]; ok {
			return
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	for _, port := range incoming {
		appendPort(port)
	}
	for _, port := range strings.Split(existing, ",") {
		appendPort(port)
	}
	return strings.Join(ports, ",")
}
