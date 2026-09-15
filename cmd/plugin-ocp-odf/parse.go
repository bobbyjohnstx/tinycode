package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type storageClusterInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Version   string `json:"version,omitempty"`
}

type cephHealthInfo struct {
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Health    string         `json:"health"`
	Message   string         `json:"message,omitempty"`
	Phase     string         `json:"phase"`
	Version   string         `json:"version,omitempty"`
	Capacity  *cephCapacity  `json:"capacity,omitempty"`
	MonCount  int            `json:"monCount,omitempty"`
	OSDStatus *osdStatusInfo `json:"osdStatus,omitempty"`
}

type cephCapacity struct {
	Total    string `json:"total,omitempty"`
	Used     string `json:"used,omitempty"`
	Available string `json:"available,omitempty"`
}

type osdStatusInfo struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	In      int `json:"in"`
}

type poolInfo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Replication int    `json:"replication,omitempty"`
	ECK         int    `json:"erasureCodingK,omitempty"`
	ECM         int    `json:"erasureCodingM,omitempty"`
	Phase       string `json:"phase,omitempty"`
}

type pvcInfo struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	Capacity     string `json:"capacity,omitempty"`
	StorageClass string `json:"storageClass"`
	AccessModes  string `json:"accessModes,omitempty"`
	VolumeName   string `json:"volumeName,omitempty"`
}

type bucketInfo struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Phase        string `json:"phase"`
	StorageClass string `json:"storageClass,omitempty"`
	BucketName   string `json:"bucketName,omitempty"`
}

type storageClassInfo struct {
	Name         string `json:"name"`
	Provisioner  string `json:"provisioner"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	VolumeBinding string `json:"volumeBindingMode,omitempty"`
	Default      bool   `json:"default,omitempty"`
}

type odfSummary struct {
	Phase      string `json:"phase"`
	CephHealth string `json:"cephHealth"`
	Capacity   string `json:"capacity,omitempty"`
}

func parseStorageClusters(raw string) ([]storageClusterInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Version string `json:"version,omitempty"`
			} `json:"spec"`
			Status *struct {
				Phase string `json:"phase"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var results []storageClusterInfo
	for _, item := range list.Items {
		sc := storageClusterInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Version:   item.Spec.Version,
		}
		if item.Status != nil {
			sc.Phase = item.Status.Phase
		} else {
			sc.Phase = "Unknown"
		}
		results = append(results, sc)
	}
	return results, nil
}

func parseCephClusters(raw string) ([]cephHealthInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				CephVersion struct {
					Image string `json:"image,omitempty"`
				} `json:"cephVersion"`
				Mon struct {
					Count int `json:"count"`
				} `json:"mon"`
			} `json:"spec"`
			Status *struct {
				Phase   string `json:"phase"`
				CephStatus *struct {
					Health  string `json:"health"`
					Details string `json:"details,omitempty"`
					LastChecked string `json:"lastChecked,omitempty"`
				} `json:"ceph,omitempty"`
				CephStorage *struct {
					Capacity struct {
						TotalBytes     json.Number `json:"totalBytes,omitempty"`
						UsedBytes      json.Number `json:"usedBytes,omitempty"`
						AvailableBytes json.Number `json:"availableBytes,omitempty"`
					} `json:"capacity,omitempty"`
					OSD struct {
						StoreCount map[string]int `json:"storeCount,omitempty"`
					} `json:"osd,omitempty"`
					DeviceClasses []struct {
						Name string `json:"name"`
					} `json:"deviceClasses,omitempty"`
				} `json:"storage,omitempty"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}

	var results []cephHealthInfo
	for _, item := range list.Items {
		info := cephHealthInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			MonCount:  item.Spec.Mon.Count,
		}
		if item.Spec.CephVersion.Image != "" {
			parts := strings.Split(item.Spec.CephVersion.Image, ":")
			if len(parts) > 1 {
				info.Version = parts[len(parts)-1]
			}
		}
		if item.Status != nil {
			info.Phase = item.Status.Phase
			if item.Status.CephStatus != nil {
				info.Health = item.Status.CephStatus.Health
				info.Message = item.Status.CephStatus.Details
			}
			if item.Status.CephStorage != nil {
				cap := &cephCapacity{}
				if t := item.Status.CephStorage.Capacity.TotalBytes; t.String() != "" {
					cap.Total = formatBytes(t)
				}
				if u := item.Status.CephStorage.Capacity.UsedBytes; u.String() != "" {
					cap.Used = formatBytes(u)
				}
				if a := item.Status.CephStorage.Capacity.AvailableBytes; a.String() != "" {
					cap.Available = formatBytes(a)
				}
				if cap.Total != "" || cap.Used != "" {
					info.Capacity = cap
				}
				total := 0
				for _, count := range item.Status.CephStorage.OSD.StoreCount {
					total += count
				}
				if total > 0 {
					info.OSDStatus = &osdStatusInfo{Total: total, Up: total, In: total}
				}
			}
		} else {
			info.Phase = "Unknown"
			info.Health = "Unknown"
		}
		results = append(results, info)
	}
	return results, nil
}

func parseBlockPools(raw string) ([]poolInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Replicated struct {
					Size int `json:"size"`
				} `json:"replicated"`
				ErasureCoded struct {
					DataChunks   int `json:"dataChunks"`
					CodingChunks int `json:"codingChunks"`
				} `json:"erasureCoded"`
			} `json:"spec"`
			Status *struct {
				Phase string `json:"phase"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var results []poolInfo
	for _, item := range list.Items {
		p := poolInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Kind:      "CephBlockPool",
		}
		if item.Spec.Replicated.Size > 0 {
			p.Replication = item.Spec.Replicated.Size
		}
		if item.Spec.ErasureCoded.DataChunks > 0 {
			p.ECK = item.Spec.ErasureCoded.DataChunks
			p.ECM = item.Spec.ErasureCoded.CodingChunks
		}
		if item.Status != nil {
			p.Phase = item.Status.Phase
		}
		results = append(results, p)
	}
	return results, nil
}

func parseFilesystems(raw string) ([]poolInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				DataPools []struct {
					Replicated struct {
						Size int `json:"size"`
					} `json:"replicated"`
				} `json:"dataPools"`
			} `json:"spec"`
			Status *struct {
				Phase string `json:"phase"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var results []poolInfo
	for _, item := range list.Items {
		p := poolInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Kind:      "CephFilesystem",
		}
		if len(item.Spec.DataPools) > 0 && item.Spec.DataPools[0].Replicated.Size > 0 {
			p.Replication = item.Spec.DataPools[0].Replicated.Size
		}
		if item.Status != nil {
			p.Phase = item.Status.Phase
		}
		results = append(results, p)
	}
	return results, nil
}

func parsePVCs(raw string, odfClasses map[string]bool) ([]pvcInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				StorageClassName string   `json:"storageClassName"`
				AccessModes      []string `json:"accessModes"`
				VolumeName       string   `json:"volumeName"`
			} `json:"spec"`
			Status struct {
				Phase    string `json:"phase"`
				Capacity struct {
					Storage string `json:"storage"`
				} `json:"capacity"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var results []pvcInfo
	for _, item := range list.Items {
		if !odfClasses[item.Spec.StorageClassName] {
			continue
		}
		pvc := pvcInfo{
			Name:         item.Metadata.Name,
			Namespace:    item.Metadata.Namespace,
			Status:       item.Status.Phase,
			Capacity:     item.Status.Capacity.Storage,
			StorageClass: item.Spec.StorageClassName,
			VolumeName:   item.Spec.VolumeName,
		}
		if len(item.Spec.AccessModes) > 0 {
			pvc.AccessModes = strings.Join(item.Spec.AccessModes, ",")
		}
		results = append(results, pvc)
	}
	return results, nil
}

func parseBuckets(raw string) ([]bucketInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				StorageClassName string `json:"storageClassName"`
			} `json:"spec"`
			Status *struct {
				Phase string `json:"phase"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var results []bucketInfo
	for _, item := range list.Items {
		b := bucketInfo{
			Name:         item.Metadata.Name,
			Namespace:    item.Metadata.Namespace,
			StorageClass: item.Spec.StorageClassName,
		}
		if item.Status != nil {
			b.Phase = item.Status.Phase
		} else {
			b.Phase = "Unknown"
		}
		results = append(results, b)
	}
	return results, nil
}

func parseStorageClasses(raw string) ([]storageClassInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name        string            `json:"name"`
				Annotations map[string]string `json:"annotations,omitempty"`
			} `json:"metadata"`
			Provisioner       string `json:"provisioner"`
			ReclaimPolicy     string `json:"reclaimPolicy"`
			VolumeBindingMode string `json:"volumeBindingMode,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	odfProvisioners := map[string]bool{
		"openshift-storage.rbd.csi.ceph.com":    true,
		"openshift-storage.cephfs.csi.ceph.com":  true,
		"openshift-storage.noobaa.io/obc":        true,
		"rook-ceph.rbd.csi.ceph.com":             true,
		"rook-ceph.cephfs.csi.ceph.com":          true,
	}
	var results []storageClassInfo
	for _, item := range list.Items {
		if !odfProvisioners[item.Provisioner] {
			continue
		}
		sc := storageClassInfo{
			Name:          item.Metadata.Name,
			Provisioner:   item.Provisioner,
			ReclaimPolicy: item.ReclaimPolicy,
			VolumeBinding: item.VolumeBindingMode,
		}
		if item.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			sc.Default = true
		}
		results = append(results, sc)
	}
	return results, nil
}

type nodeResourceInfo struct {
	Name               string `json:"name"`
	CPUCapacity        string `json:"cpuCapacity"`
	CPUAllocatable     string `json:"cpuAllocatable"`
	MemoryCapacity     string `json:"memoryCapacity"`
	MemoryAllocatable  string `json:"memoryAllocatable"`
	StorageCapacity    string `json:"storageCapacity"`
	StorageAllocatable string `json:"storageAllocatable"`
}

func parseNodeResources(raw json.RawMessage) ([]nodeResourceInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Capacity    map[string]string `json:"capacity"`
				Allocatable map[string]string `json:"allocatable"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	var nodes []nodeResourceInfo
	for _, item := range list.Items {
		n := nodeResourceInfo{
			Name:               item.Metadata.Name,
			CPUCapacity:        item.Status.Capacity["cpu"],
			CPUAllocatable:     item.Status.Allocatable["cpu"],
			MemoryCapacity:     item.Status.Capacity["memory"],
			MemoryAllocatable:  item.Status.Allocatable["memory"],
			StorageCapacity:    item.Status.Capacity["ephemeral-storage"],
			StorageAllocatable: item.Status.Allocatable["ephemeral-storage"],
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func formatBytes(n json.Number) string {
	val, err := n.Int64()
	if err != nil {
		return n.String()
	}
	switch {
	case val >= 1<<40:
		return fmt.Sprintf("%.1f TiB", float64(val)/float64(1<<40))
	case val >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(val)/float64(1<<30))
	case val >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(val)/float64(1<<20))
	default:
		return fmt.Sprintf("%d B", val)
	}
}
