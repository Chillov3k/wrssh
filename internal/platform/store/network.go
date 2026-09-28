package store

import (
	"errors"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NetworkScanNodeRecord is a pscan-discovered host stored in the control-plane
// database. It is only used for projects without an isolated runtime; runtime
// projects keep their network map in the per-project postgres container.
type NetworkScanNodeRecord struct {
	ID          uint       `json:"id"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Project     string     `gorm:"uniqueIndex:idx_network_scan_nodes_project_ip,priority:1;size:128" json:"project"`
	IP          string     `gorm:"uniqueIndex:idx_network_scan_nodes_project_ip,priority:2;size:64" json:"ip"`
	Hostname    string     `gorm:"size:255" json:"hostname"`
	OS          string     `gorm:"size:32" json:"os"`
	Ports       string     `gorm:"size:2048" json:"ports"`
	FirstSeenAt *time.Time `json:"firstSeenAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
}

// NetworkScanEdgeRecord is a "scanned from" link between two discovered IPs.
type NetworkScanEdgeRecord struct {
	ID          uint       `json:"id"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Project     string     `gorm:"uniqueIndex:idx_network_scan_edges,priority:1;size:128" json:"project"`
	FromIP      string     `gorm:"uniqueIndex:idx_network_scan_edges,priority:2;size:64" json:"fromIp"`
	ToIP        string     `gorm:"uniqueIndex:idx_network_scan_edges,priority:3;size:64" json:"toIp"`
	FirstSeenAt *time.Time `json:"firstSeenAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
}

// UpsertNetworkScanForProject records one pscan run for a project in the
// control-plane database.
func (s *Store) UpsertNetworkScanForProject(project, sourceIP string, discoveries []networkmap.Discovery) error {
	projectName := DisplayProjectName(project)
	sourceIP = strings.TrimSpace(sourceIP)
	if sourceIP == "" {
		return errors.New("scan source ip is required")
	}
	if len(discoveries) == 0 {
		return nil
	}

	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, discovery := range discoveries {
			ip := strings.TrimSpace(discovery.IP)
			if ip == "" {
				continue
			}

			ports := make([]string, 0, len(discovery.Ports))
			for _, port := range discovery.Ports {
				ports = append(ports, port.Label())
			}

			var node NetworkScanNodeRecord
			err := tx.Where("project = ? AND ip = ?", projectName, ip).Limit(1).Find(&node).Error
			if err != nil {
				return err
			}

			if node.IP == "" {
				node = NetworkScanNodeRecord{
					Project:     projectName,
					IP:          ip,
					Hostname:    strings.TrimSpace(discovery.Hostname),
					OS:          strings.TrimSpace(discovery.OS),
					Ports:       strings.Join(ports, ","),
					FirstSeenAt: ptrTime(now),
					LastSeenAt:  ptrTime(now),
				}
				if err := tx.Create(&node).Error; err != nil {
					return err
				}
			} else {
				updates := map[string]any{
					"ports":        mergeNetworkPortCSV(node.Ports, ports),
					"last_seen_at": now,
				}
				if strings.TrimSpace(node.Hostname) == "" && strings.TrimSpace(discovery.Hostname) != "" {
					updates["hostname"] = strings.TrimSpace(discovery.Hostname)
				}
				if strings.TrimSpace(node.OS) == "" && strings.TrimSpace(discovery.OS) != "" {
					updates["os"] = strings.TrimSpace(discovery.OS)
				}
				if err := tx.Model(&NetworkScanNodeRecord{}).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
					return err
				}
			}

			if ip == sourceIP {
				continue
			}

			edge := NetworkScanEdgeRecord{
				Project:     projectName,
				FromIP:      sourceIP,
				ToIP:        ip,
				FirstSeenAt: ptrTime(now),
				LastSeenAt:  ptrTime(now),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "project"}, {Name: "from_ip"}, {Name: "to_ip"}},
				DoUpdates: clause.AssignmentColumns([]string{"last_seen_at", "updated_at"}),
			}).Create(&edge).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ListNetworkScanForProject returns the stored scan discoveries of a project.
func (s *Store) ListNetworkScanForProject(project string) ([]NetworkScanNodeRecord, []NetworkScanEdgeRecord, error) {
	projectName := DisplayProjectName(project)

	var nodes []NetworkScanNodeRecord
	if err := s.db.Where("project = ?", projectName).Order("ip asc").Find(&nodes).Error; err != nil {
		return nil, nil, err
	}

	var edges []NetworkScanEdgeRecord
	if err := s.db.Where("project = ?", projectName).Order("from_ip asc, to_ip asc").Find(&edges).Error; err != nil {
		return nil, nil, err
	}

	return nodes, edges, nil
}

// ClearNetworkScanForProject removes all stored scan discoveries of a project.
func (s *Store) ClearNetworkScanForProject(project string) error {
	projectName := DisplayProjectName(project)

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project = ?", projectName).Delete(&NetworkScanEdgeRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("project = ?", projectName).Delete(&NetworkScanNodeRecord{}).Error
	})
}

func mergeNetworkPortCSV(existing string, incoming []string) string {
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
