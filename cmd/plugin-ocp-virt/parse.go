package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type vmInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Node      string `json:"node,omitempty"`
	IP        string `json:"ip,omitempty"`
	OS        string `json:"os,omitempty"`
	CPUs      int    `json:"cpus,omitempty"`
	Memory    string `json:"memory,omitempty"`
}

type dvInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Progress  string `json:"progress,omitempty"`
	Size      string `json:"size,omitempty"`
	Source    string `json:"source,omitempty"`
}

type vmSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Error   int `json:"error"`
	Other   int `json:"other"`
}

func classifyVMStatus(printableStatus string) string {
	s := strings.ToLower(printableStatus)
	switch {
	case s == "running":
		return "running"
	case s == "stopped" || s == "poweroff" || s == "shutoff":
		return "stopped"
	case strings.Contains(s, "error") || strings.Contains(s, "crash") || strings.Contains(s, "fail"):
		return "error"
	default:
		return "other"
	}
}

func parseVMList(raw json.RawMessage) ([]vmInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						Domain struct {
							CPU struct {
								Cores int `json:"cores"`
							} `json:"cpu"`
							Resources struct {
								Requests map[string]string `json:"requests"`
							} `json:"resources"`
						} `json:"domain"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
			Status *struct {
				PrintableStatus string `json:"printableStatus"`
				Ready           bool   `json:"ready"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}

	var vms []vmInfo
	for _, item := range list.Items {
		vm := vmInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			CPUs:      item.Spec.Template.Spec.Domain.CPU.Cores,
		}
		if mem, ok := item.Spec.Template.Spec.Domain.Resources.Requests["memory"]; ok {
			vm.Memory = mem
		}
		if item.Status != nil {
			vm.Status = item.Status.PrintableStatus
			if item.Status.Ready {
				vm.Ready = "true"
			} else {
				vm.Ready = "false"
			}
		} else {
			vm.Status = "Unknown"
			vm.Ready = "false"
		}
		vms = append(vms, vm)
	}
	return vms, nil
}

func parseVMDetail(raw json.RawMessage) (map[string]any, error) {
	var vm struct {
		Metadata struct {
			Name              string `json:"name"`
			Namespace         string `json:"namespace"`
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Domain struct {
						CPU struct {
							Cores   int    `json:"cores"`
							Sockets int    `json:"sockets"`
							Threads int    `json:"threads"`
							Model   string `json:"model,omitempty"`
						} `json:"cpu"`
						Resources struct {
							Requests map[string]string `json:"requests"`
							Limits   map[string]string `json:"limits"`
						} `json:"resources"`
						Devices struct {
							Disks []struct {
								Name string `json:"name"`
								Disk *struct {
									Bus string `json:"bus"`
								} `json:"disk,omitempty"`
								CDRom *struct{} `json:"cdrom,omitempty"`
							} `json:"disks,omitempty"`
						} `json:"devices"`
					} `json:"domain"`
					Volumes []struct {
						Name                  string                                        `json:"name"`
						DataVolume            *struct{ Name string `json:"name"` }           `json:"dataVolume,omitempty"`
						PersistentVolumeClaim *struct{ ClaimName string `json:"claimName"` } `json:"persistentVolumeClaim,omitempty"`
						ContainerDisk         *struct{ Image string `json:"image"` }         `json:"containerDisk,omitempty"`
						CloudInitNoCloud      *struct{}                                      `json:"cloudInitNoCloud,omitempty"`
						CloudInitConfigDrive  *struct{}                                      `json:"cloudInitConfigDrive,omitempty"`
					} `json:"volumes,omitempty"`
					Networks []struct {
						Name   string    `json:"name"`
						Pod    *struct{} `json:"pod,omitempty"`
						Multus *struct {
							NetworkName string `json:"networkName"`
						} `json:"multus,omitempty"`
					} `json:"networks,omitempty"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
		Status *struct {
			PrintableStatus string `json:"printableStatus"`
			Ready           bool   `json:"ready"`
			Conditions      []struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Reason  string `json:"reason,omitempty"`
				Message string `json:"message,omitempty"`
			} `json:"conditions,omitempty"`
			GuestOSInfo *struct {
				Name          string `json:"name"`
				KernelRelease string `json:"kernelRelease"`
				ID            string `json:"id"`
				Version       string `json:"version"`
			} `json:"guestOSInfo,omitempty"`
			Interfaces []struct {
				Name string   `json:"name"`
				IP   string   `json:"ipAddress"`
				IPs  []string `json:"ipAddresses,omitempty"`
				MAC  string   `json:"mac"`
			} `json:"interfaces,omitempty"`
			NodeName string `json:"nodeName,omitempty"`
		} `json:"status,omitempty"`
	}
	if err := json.Unmarshal(raw, &vm); err != nil {
		return nil, err
	}

	detail := map[string]any{
		"name":      vm.Metadata.Name,
		"namespace": vm.Metadata.Namespace,
		"created":   vm.Metadata.CreationTimestamp,
	}

	cpu := map[string]any{"cores": vm.Spec.Template.Spec.Domain.CPU.Cores}
	if vm.Spec.Template.Spec.Domain.CPU.Sockets > 0 {
		cpu["sockets"] = vm.Spec.Template.Spec.Domain.CPU.Sockets
	}
	if vm.Spec.Template.Spec.Domain.CPU.Threads > 0 {
		cpu["threads"] = vm.Spec.Template.Spec.Domain.CPU.Threads
	}
	detail["cpu"] = cpu

	if len(vm.Spec.Template.Spec.Domain.Resources.Requests) > 0 {
		detail["resourceRequests"] = vm.Spec.Template.Spec.Domain.Resources.Requests
	}

	var disks []map[string]string
	for _, d := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		disk := map[string]string{"name": d.Name}
		if d.Disk != nil {
			disk["type"] = "disk"
			disk["bus"] = d.Disk.Bus
		} else if d.CDRom != nil {
			disk["type"] = "cdrom"
		}
		disks = append(disks, disk)
	}
	if len(disks) > 0 {
		detail["disks"] = disks
	}

	var volumes []map[string]string
	for _, v := range vm.Spec.Template.Spec.Volumes {
		vol := map[string]string{"name": v.Name}
		switch {
		case v.DataVolume != nil:
			vol["type"] = "dataVolume"
			vol["source"] = v.DataVolume.Name
		case v.PersistentVolumeClaim != nil:
			vol["type"] = "pvc"
			vol["source"] = v.PersistentVolumeClaim.ClaimName
		case v.ContainerDisk != nil:
			vol["type"] = "containerDisk"
			vol["source"] = v.ContainerDisk.Image
		case v.CloudInitNoCloud != nil:
			vol["type"] = "cloudInitNoCloud"
		case v.CloudInitConfigDrive != nil:
			vol["type"] = "cloudInitConfigDrive"
		}
		volumes = append(volumes, vol)
	}
	if len(volumes) > 0 {
		detail["volumes"] = volumes
	}

	var networks []map[string]string
	for _, n := range vm.Spec.Template.Spec.Networks {
		net := map[string]string{"name": n.Name}
		if n.Pod != nil {
			net["type"] = "pod"
		} else if n.Multus != nil {
			net["type"] = "multus"
			net["networkName"] = n.Multus.NetworkName
		}
		networks = append(networks, net)
	}
	if len(networks) > 0 {
		detail["networks"] = networks
	}

	if vm.Status != nil {
		detail["status"] = vm.Status.PrintableStatus
		detail["ready"] = vm.Status.Ready
		if vm.Status.NodeName != "" {
			detail["node"] = vm.Status.NodeName
		}
		if vm.Status.GuestOSInfo != nil {
			os := map[string]string{}
			if vm.Status.GuestOSInfo.Name != "" {
				os["name"] = vm.Status.GuestOSInfo.Name
			}
			if vm.Status.GuestOSInfo.ID != "" {
				os["id"] = vm.Status.GuestOSInfo.ID
			}
			if vm.Status.GuestOSInfo.Version != "" {
				os["version"] = vm.Status.GuestOSInfo.Version
			}
			if vm.Status.GuestOSInfo.KernelRelease != "" {
				os["kernel"] = vm.Status.GuestOSInfo.KernelRelease
			}
			if len(os) > 0 {
				detail["guestOS"] = os
			}
		}
		if len(vm.Status.Interfaces) > 0 {
			var ifaces []map[string]any
			for _, iface := range vm.Status.Interfaces {
				i := map[string]any{
					"name": iface.Name,
					"mac":  iface.MAC,
				}
				if iface.IP != "" {
					i["ip"] = iface.IP
				}
				if len(iface.IPs) > 0 {
					i["ips"] = iface.IPs
				}
				ifaces = append(ifaces, i)
			}
			detail["interfaces"] = ifaces
		}
		if len(vm.Status.Conditions) > 0 {
			var conditions []map[string]string
			for _, c := range vm.Status.Conditions {
				cond := map[string]string{
					"type":   c.Type,
					"status": c.Status,
				}
				if c.Reason != "" {
					cond["reason"] = c.Reason
				}
				if c.Message != "" {
					cond["message"] = c.Message
				}
				conditions = append(conditions, cond)
			}
			detail["conditions"] = conditions
		}
	}

	return detail, nil
}

type nodeCapacityInfo struct {
	Name               string   `json:"name"`
	Roles              []string `json:"roles,omitempty"`
	CPUCapacity        string   `json:"cpuCapacity"`
	CPUAllocatable     string   `json:"cpuAllocatable"`
	MemoryCapacity     string   `json:"memoryCapacity"`
	MemoryAllocatable  string   `json:"memoryAllocatable"`
}

func parseNodeCapacity(raw json.RawMessage) ([]nodeCapacityInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
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
	var nodes []nodeCapacityInfo
	for _, item := range list.Items {
		n := nodeCapacityInfo{
			Name:               item.Metadata.Name,
			CPUCapacity:        item.Status.Capacity["cpu"],
			CPUAllocatable:     item.Status.Allocatable["cpu"],
			MemoryCapacity:     item.Status.Capacity["memory"],
			MemoryAllocatable:  item.Status.Allocatable["memory"],
		}
		for label := range item.Metadata.Labels {
			const prefix = "node-role.kubernetes.io/"
			if strings.HasPrefix(label, prefix) {
				n.Roles = append(n.Roles, label[len(prefix):])
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func parseDVList(raw string) ([]dvInfo, error) {
	var dvList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Source *struct {
					HTTP     *struct{ URL string `json:"url"` }                                          `json:"http,omitempty"`
					Registry *struct{ URL string `json:"url"` }                                          `json:"registry,omitempty"`
					PVC      *struct{ Name string `json:"name"`; Namespace string `json:"namespace"` }   `json:"pvc,omitempty"`
					S3       *struct{ URL string `json:"url"` }                                          `json:"s3,omitempty"`
					Blank    *struct{}                                                                    `json:"blank,omitempty"`
				} `json:"source,omitempty"`
				PVC *struct {
					Resources struct {
						Requests map[string]string `json:"requests"`
					} `json:"resources"`
				} `json:"pvc,omitempty"`
			} `json:"spec"`
			Status *struct {
				Phase    string `json:"phase"`
				Progress string `json:"progress,omitempty"`
			} `json:"status,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &dvList); err != nil {
		return nil, err
	}

	var dvs []dvInfo
	for _, item := range dvList.Items {
		dv := dvInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
		}
		if item.Status != nil {
			dv.Phase = item.Status.Phase
			dv.Progress = item.Status.Progress
		} else {
			dv.Phase = "Unknown"
		}
		if item.Spec.PVC != nil {
			if sz, ok := item.Spec.PVC.Resources.Requests["storage"]; ok {
				dv.Size = sz
			}
		}
		if item.Spec.Source != nil {
			switch {
			case item.Spec.Source.HTTP != nil:
				dv.Source = "http: " + item.Spec.Source.HTTP.URL
			case item.Spec.Source.Registry != nil:
				dv.Source = "registry: " + item.Spec.Source.Registry.URL
			case item.Spec.Source.PVC != nil:
				dv.Source = fmt.Sprintf("pvc: %s/%s", item.Spec.Source.PVC.Namespace, item.Spec.Source.PVC.Name)
			case item.Spec.Source.S3 != nil:
				dv.Source = "s3: " + item.Spec.Source.S3.URL
			case item.Spec.Source.Blank != nil:
				dv.Source = "blank"
			}
		}
		dvs = append(dvs, dv)
	}
	return dvs, nil
}
